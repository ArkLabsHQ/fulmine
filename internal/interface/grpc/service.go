package grpc_interface

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	delegatev1 "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/delegate/v1"
	fulminev1 "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/ArkLabsHQ/fulmine/internal/core/application"
	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/telemetry"
	"github.com/ArkLabsHQ/fulmine/internal/interface/grpc/handlers"
	"github.com/ArkLabsHQ/fulmine/internal/interface/grpc/interceptors"
	"github.com/ArkLabsHQ/fulmine/internal/interface/web"
	"github.com/ArkLabsHQ/fulmine/pkg/macaroon"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	grpchealth "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/encoding/protojson"
)

type service struct {
	cfg                Config
	appSvc             *application.Service
	delegateSvc        *application.DelegateService
	httpServer         *http.Server
	delegateHTTPServer *http.Server
	grpcServer         *grpc.Server
	delegateGrpcServer *grpc.Server
	unlockerSvc        ports.Unlocker
	macaroonSvc        macaroon.Service
	arkServer          string
	appStopCh          chan struct{}
	feStopCh           chan struct{}
	otelShutdown       func()
	pyroscopeShutdown  func()
}

func NewService(
	cfg Config,
	appSvc *application.Service,
	delegateSvc *application.DelegateService,
	unlockerSvc ports.Unlocker,
	sentryEnabled bool,
	macaroonSvc macaroon.Service,
	arkServer string,
	otelCollectorEndpoint string,
	otelPushInterval int64,
	pyroscopeServerURL string,
) (*service, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %s", err)
	}

	appStopCh := make(chan struct{}, 1)
	feStopCh := make(chan struct{}, 1)

	grpcConfig := []grpc.ServerOption{
		interceptors.MacaroonAuthInterceptor(macaroonSvc),
		interceptors.MacaroonStreamAuthInterceptor(macaroonSvc),
		interceptors.UnaryInterceptor(sentryEnabled),
		interceptors.StreamInterceptor(sentryEnabled),
	}

	// Initialize OTel and Pyroscope telemetry
	var otelShutdown, pyroscopeShutdown func()

	if otelCollectorEndpoint != "" {
		log.AddHook(telemetry.NewOTelHook())

		pushInterval := time.Duration(otelPushInterval) * time.Second
		shutdown, err := telemetry.InitOtelSDK(
			context.Background(),
			otelCollectorEndpoint,
			pushInterval,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize otel sdk: %s", err)
		}
		otelShutdown = shutdown

		otelHandler := otelgrpc.NewServerHandler(
			otelgrpc.WithTracerProvider(otel.GetTracerProvider()),
		)
		grpcConfig = append(grpcConfig, grpc.StatsHandler(otelHandler))

		if pyroscopeServerURL != "" {
			shutdown, err := telemetry.InitPyroscope(pyroscopeServerURL)
			if err != nil {
				return nil, fmt.Errorf("failed to initialize pyroscope: %s", err)
			}
			pyroscopeShutdown = shutdown
		}
	}

	if cfg.WithTLS {
		return nil, fmt.Errorf("tls termination not supported yet")
	}
	creds := insecure.NewCredentials()
	if !cfg.insecure() {
		creds = credentials.NewTLS(cfg.tlsConfig())
	}
	grpcConfig = append(grpcConfig, grpc.Creds(creds))

	grpcServer := grpc.NewServer(grpcConfig...)

	walletHandler := handlers.NewWalletHandler(appSvc, unlockerSvc)
	fulminev1.RegisterWalletServiceServer(grpcServer, walletHandler)

	serviceHandler := handlers.NewServiceHandler(appSvc)
	fulminev1.RegisterServiceServer(grpcServer, serviceHandler)

	notificationHandler := handlers.NewNotificationHandler(appSvc, appStopCh)
	fulminev1.RegisterNotificationServiceServer(grpcServer, notificationHandler)

	healthHandler := handlers.NewHealthHandler(appSvc)
	grpchealth.RegisterHealthServer(grpcServer, healthHandler)

	reflection.Register(grpcServer)

	gatewayCreds := insecure.NewCredentials()
	if !cfg.insecure() {
		gatewayCreds = credentials.NewTLS(&tls.Config{
			InsecureSkipVerify: true, // #nosec
		})
	}
	gatewayOpts := grpc.WithTransportCredentials(gatewayCreds)
	conn, err := grpc.NewClient(cfg.gatewayAddress(), gatewayOpts)
	if err != nil {
		return nil, err
	}

	authHeaderMatcher := func(key string) (string, bool) {
		switch key {
		case "X-Macaroon":
			return "macaroon", true
		default:
			return key, false
		}
	}
	healthzHandler := grpchealth.NewHealthClient(conn)
	gwmux := runtime.NewServeMux(
		runtime.WithIncomingHeaderMatcher(authHeaderMatcher),
		runtime.WithHealthzEndpoint(healthzHandler),
		runtime.WithMarshalerOption("application/json+pretty", &runtime.JSONPb{
			MarshalOptions: protojson.MarshalOptions{
				Indent:    "  ",
				Multiline: true,
			},
			UnmarshalOptions: protojson.UnmarshalOptions{
				DiscardUnknown: true,
			},
		}),
	)
	// nolint
	gwmux.HandlePath("GET", "/healthz", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		resp, err := healthzHandler.Check(r.Context(), &grpchealth.HealthCheckRequest{Service: "fulmine"})
		if err != nil {
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
			return
		}

		switch resp.Status {
		case grpchealth.HealthCheckResponse_SERVING:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		case grpchealth.HealthCheckResponse_NOT_SERVING:
			http.Error(w, "unhealthy", http.StatusServiceUnavailable)
		case grpchealth.HealthCheckResponse_SERVICE_UNKNOWN:
			http.Error(w, "unknown service", http.StatusNotFound)
		default:
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		}
	})
	ctx := context.Background()
	if err := fulminev1.RegisterServiceHandler(ctx, gwmux, conn); err != nil {
		return nil, err
	}
	if err := fulminev1.RegisterWalletServiceHandler(ctx, gwmux, conn); err != nil {
		return nil, err
	}
	if err := fulminev1.RegisterNotificationServiceHandler(ctx, gwmux, conn); err != nil {
		return nil, err
	}

	feHandler := web.NewService(appSvc, feStopCh, sentryEnabled, arkServer, unlockerSvc, delegateSvc != nil)

	mux := http.NewServeMux()
	mux.Handle("/", feHandler)
	mux.Handle("/api/", http.StripPrefix("/api", gwmux))
	mux.Handle("/static/", feHandler)

	httpServer := &http.Server{
		Addr:      cfg.httpAddress(),
		Handler:   mux,
		TLSConfig: cfg.tlsConfig(),
	}
	if cfg.insecure() {
		httpServer.Protocols = h2cProtocols()
	}

	// Setup server for the delegate if enabled
	var delegateHTTPServer *http.Server
	var delegateGrpcServer *grpc.Server
	if delegateSvc != nil {
		grpcConfig := []grpc.ServerOption{
			interceptors.UnaryInterceptor(false), interceptors.StreamInterceptor(false),
		}
		delegateGrpcServer = grpc.NewServer(grpcConfig...)
		delegateHandler := handlers.NewDelegateHandler(delegateSvc)
		delegatev1.RegisterDelegateServiceServer(delegateGrpcServer, delegateHandler)

		conn, err := grpc.NewClient(cfg.delegateGatewayAddress(), gatewayOpts)
		if err != nil {
			return nil, err
		}
		delegateGwmux := runtime.NewServeMux(
			runtime.WithIncomingHeaderMatcher(authHeaderMatcher),
			runtime.WithMarshalerOption("application/json+pretty", &runtime.JSONPb{
				MarshalOptions: protojson.MarshalOptions{
					Indent:    "  ",
					Multiline: true,
				},
				UnmarshalOptions: protojson.UnmarshalOptions{
					DiscardUnknown: true,
				},
			}),
		)

		if err := delegatev1.RegisterDelegateServiceHandler(ctx, delegateGwmux, conn); err != nil {
			return nil, err
		}

		grpcGateway := http.Handler(delegateGwmux)

		handler := router(delegateGrpcServer, grpcGateway)
		mux := http.NewServeMux()

		mux.Handle("/", handler)

		delegateHTTPServer = &http.Server{
			Addr:      cfg.delegateAddress(),
			Handler:   mux,
			TLSConfig: cfg.tlsConfig(),
		}
		if cfg.insecure() {
			delegateHTTPServer.Protocols = h2cProtocols()
		}
	}

	svc := &service{
		cfg:                cfg,
		appSvc:             appSvc,
		delegateSvc:        delegateSvc,
		httpServer:         httpServer,
		delegateHTTPServer: delegateHTTPServer,
		grpcServer:         grpcServer,
		delegateGrpcServer: delegateGrpcServer,
		unlockerSvc:        unlockerSvc,
		macaroonSvc:        macaroonSvc,
		arkServer:          arkServer,
		appStopCh:          appStopCh,
		feStopCh:           feStopCh,
		otelShutdown:       otelShutdown,
		pyroscopeShutdown:  pyroscopeShutdown,
	}

	if macaroonSvc != nil {
		go svc.listenToWalletUpdates()
	}

	return svc, nil
}

func (s *service) Start() error {
	listener, err := net.Listen("tcp", s.cfg.grpcAddress())
	if err != nil {
		return err
	}
	// nolint:all
	go s.grpcServer.Serve(listener)
	log.Infof("started GRPC server at %s", s.cfg.grpcAddress())

	if s.cfg.insecure() {
		// nolint:all
		go s.httpServer.ListenAndServe()
	} else {
		// nolint:all
		go s.httpServer.ListenAndServeTLS("", "")
	}
	log.Infof("started HTTP server at %s", s.cfg.httpAddress())

	if s.delegateGrpcServer != nil {
		if s.cfg.insecure() {
			// nolint:all
			go s.delegateHTTPServer.ListenAndServe()
		} else {
			// nolint:all
			go s.delegateHTTPServer.ListenAndServeTLS("", "")
		}
		log.Infof("started Delegate server at %s", s.cfg.delegateAddress())
	}

	// The listeners above are already accepting connections while the wallet is
	// created and unlocked below. That window is deliberate: it lets /healthz
	// answer NOT_SERVING instead of refusing connections during the auto-init
	// retries, and any request arriving before the wallet is ready is rejected
	// by the readiness gate. If a step below fails, Start returns and the
	// process exits, cutting whatever arrived in the meantime.
	if s.cfg.AutoInit {
		if err := s.autoInit(); err != nil {
			return fmt.Errorf("auto-init failed: %w", err)
		}
	}

	if s.unlockerSvc != nil {
		if err := s.autoUnlock(); err != nil {
			if s.cfg.AutoInit {
				// Auto-init promises a wallet that ends up unlocked: a created-but-locked
				// delegate is not operational, and a restart retries only the unlock.
				return fmt.Errorf("auto-unlock failed: %w", err)
			}
			log.Warnf("failed to auto-unlock: %v", err)
		}
	}

	if s.cfg.AutoInit {
		if s.cfg.AutoInitMnemonic != "" {
			if err := s.verifyConfiguredMnemonic(); err != nil {
				return err
			}
		}
		// Nothing reads the configured mnemonic past this point: drop the
		// reference so it is not retained for the process lifetime. When it came
		// from FULMINE_MNEMONIC the value still lives in the process environment,
		// so this bounds the live set rather than erasing the secret — which is
		// why FULMINE_MNEMONIC_FILE_PATH is the recommended way to supply it.
		s.cfg.AutoInitMnemonic = ""
	}

	return nil
}

// autoUnlock attempts to unlock the wallet automatically using the unlocker service
func (s *service) autoUnlock() error {
	ctx := context.Background()

	if !s.appSvc.IsInitialized() {
		log.Debug("wallet not initialized, skipping auto unlock")
		return nil
	}

	password, err := s.unlockerSvc.GetPassword(ctx)
	if err != nil {
		return fmt.Errorf("failed to get password: %s", err)
	}

	if err := s.appSvc.UnlockNode(ctx, password); err != nil {
		return fmt.Errorf("failed to auto unlock: %s", err)
	}

	log.Info("wallet auto unlocked")
	return nil
}

func (s *service) Stop() {
	s.appStopCh <- struct{}{}
	s.feStopCh <- struct{}{}

	s.grpcServer.GracefulStop()
	log.Info("stopped GRPC server")

	// nolint:all
	s.httpServer.Shutdown(context.Background())
	log.Info("stopped HTTP server")

	if s.delegateGrpcServer != nil {
		s.delegateSvc.Stop()

		s.delegateGrpcServer.Stop()

		// nolint:all
		s.delegateHTTPServer.Shutdown(context.Background())
		log.Info("stopped Delegate server")
	}

	if s.pyroscopeShutdown != nil {
		s.pyroscopeShutdown()
	}
	if s.otelShutdown != nil {
		s.otelShutdown()
	}
}

func (s *service) listenToWalletUpdates() {
	ctx := context.Background()
	for update := range s.appSvc.GetWalletUpdates() {
		switch update.Type {
		case application.WalletInit, application.WalletUnlock:
			if err := s.macaroonSvc.Unlock(ctx, update.Password); err != nil {
				log.WithError(err).Fatal("failed to setup macaroon service")
			}
			if err := s.macaroonSvc.Generate(ctx); err != nil {
				log.WithError(err).Fatal("failed to generate macaroons")
			}
		case application.WalletReset:
			if err := s.macaroonSvc.Reset(ctx); err != nil {
				log.WithError(err).Fatal("failed to reset macaroon service")
			}
		}
	}
}

// h2cProtocols enables cleartext HTTP/2 (with HTTP/1 fallback), replacing the
// deprecated h2c.NewHandler wrapper.
func h2cProtocols() *http.Protocols {
	p := new(http.Protocols)
	p.SetHTTP1(true)
	p.SetUnencryptedHTTP2(true)
	return p
}

func router(
	grpcServer *grpc.Server, grpcGateway http.Handler,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isOptionRequest(r) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Headers", "*")
			w.Header().Add("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
			return
		}

		if isHttpRequest(r) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Headers", "*")
			w.Header().Add("Access-Control-Allow-Methods", "POST, GET, OPTIONS")

			grpcGateway.ServeHTTP(w, r)
			return
		}
		grpcServer.ServeHTTP(w, r)
	})
}

func isOptionRequest(req *http.Request) bool {
	return req.Method == http.MethodOptions
}

func isHttpRequest(req *http.Request) bool {
	return req.Method == http.MethodGet ||
		strings.Contains(req.Header.Get("Content-Type"), "application/json")
}
