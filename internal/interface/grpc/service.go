package grpc_interface

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
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
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	grpchealth "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/encoding/protojson"
)

type service struct {
	cfg                 Config
	appSvc              *application.Service
	delegatorSvc        *application.DelegatorService
	httpServer          *http.Server
	delegatorHTTPServer *http.Server
	grpcServer          *grpc.Server
	delegatorGrpcServer *grpc.Server
	delegatorConn       *grpc.ClientConn
	unlockerSvc         ports.Unlocker
	macaroonSvc         macaroon.Service
	appStopCh           chan struct{}
	feStopCh            chan struct{}
	otelShutdown        func()
	pyroscopeShutdown   func()
}

func NewService(
	cfg Config,
	appSvc *application.Service,
	delegatorSvc *application.DelegatorService,
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

	walletHandler := handlers.NewWalletHandler(appSvc)
	pb.RegisterWalletServiceServer(grpcServer, walletHandler)

	serviceHandler := handlers.NewServiceHandler(appSvc)
	pb.RegisterServiceServer(grpcServer, serviceHandler)

	notificationHandler := handlers.NewNotificationHandler(appSvc, appStopCh)
	pb.RegisterNotificationServiceServer(grpcServer, notificationHandler)

	// if a different delegator GRPC port is configured, we need a dedicated GRPC server for the delegator
	var delegatorGrpcServer *grpc.Server
	if delegatorSvc != nil {
		delegatorGrpcServer = grpc.NewServer(grpcConfig...)
		delegateHandler := handlers.NewDelegatorHandler(delegatorSvc)
		pb.RegisterDelegatorServiceServer(delegatorGrpcServer, delegateHandler)
	}

	healthHandler := handlers.NewHealthHandler(appSvc)
	grpchealth.RegisterHealthServer(grpcServer, healthHandler)

	gatewayCreds := insecure.NewCredentials()
	if !cfg.insecure() {
		gatewayCreds = credentials.NewTLS(&tls.Config{
			InsecureSkipVerify: true, // #nosec
		})
	}
	gatewayOpts := grpc.WithTransportCredentials(gatewayCreds)
	conn, err := grpc.NewClient(
		cfg.gatewayAddress(), gatewayOpts,
	)
	if err != nil {
		return nil, err
	}

	// Create separate connection to delegator server if it's on a different port
	var delegatorConn *grpc.ClientConn
	if delegatorSvc != nil {
		delegatorConn, err = grpc.NewClient(
			cfg.delegatorGatewayAddress(), gatewayOpts,
		)
		if err != nil {
			return nil, err
		}
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
	if err := pb.RegisterServiceHandler(
		ctx, gwmux, conn,
	); err != nil {
		return nil, err
	}
	if err := pb.RegisterWalletServiceHandler(
		ctx, gwmux, conn,
	); err != nil {
		return nil, err
	}
	if err := pb.RegisterNotificationServiceHandler(
		ctx, gwmux, conn,
	); err != nil {
		return nil, err
	}

	// register delegator service handler on main gateway if not using separate HTTP port
	if delegatorSvc != nil {
		if err := pb.RegisterDelegatorServiceHandler(ctx, gwmux, delegatorConn); err != nil {
			return nil, err
		}
	}

	feHandler := web.NewService(appSvc, feStopCh, sentryEnabled, arkServer)

	mux := http.NewServeMux()
	mux.Handle("/", feHandler)
	mux.Handle("/api/", http.StripPrefix("/api", gwmux))
	mux.Handle("/static/", feHandler)

	httpServerHandler := http.Handler(mux)
	if cfg.insecure() {
		httpServerHandler = h2c.NewHandler(httpServerHandler, &http2.Server{})
	}

	httpServer := &http.Server{
		Addr:      cfg.httpAddress(),
		Handler:   httpServerHandler,
		TLSConfig: cfg.tlsConfig(),
	}

	// if a different delegator HTTP port is configured, we need a dedicated HTTP server for the
	// delegator
	var delegatorHTTPServer *http.Server
	if delegatorSvc != nil {
		delegatorGwmux := runtime.NewServeMux(
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

		if err := pb.RegisterDelegatorServiceHandler(
			ctx, delegatorGwmux, delegatorConn,
		); err != nil {
			return nil, err
		}

		delegatorMux := http.NewServeMux()
		delegatorMux.Handle("/api/", http.StripPrefix("/api", delegatorGwmux))

		delegatorHTTPHandler := http.Handler(delegatorMux)
		if cfg.insecure() {
			delegatorHTTPHandler = h2c.NewHandler(delegatorHTTPHandler, &http2.Server{})
		}

		delegatorHTTPServer = &http.Server{
			Addr:      cfg.delegatorAddress(),
			Handler:   delegatorHTTPHandler,
			TLSConfig: cfg.tlsConfig(),
		}
	}

	svc := &service{
		cfg:                 cfg,
		appSvc:              appSvc,
		delegatorSvc:        delegatorSvc,
		httpServer:          httpServer,
		delegatorHTTPServer: delegatorHTTPServer,
		grpcServer:          grpcServer,
		delegatorGrpcServer: delegatorGrpcServer,
		delegatorConn:       delegatorConn,
		unlockerSvc:         unlockerSvc,
		macaroonSvc:         macaroonSvc,
		appStopCh:           appStopCh,
		feStopCh:            feStopCh,
		otelShutdown:        otelShutdown,
		pyroscopeShutdown:   pyroscopeShutdown,
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

	if s.delegatorGrpcServer != nil {
		delegatorListener, err := net.Listen("tcp", s.cfg.delegatorAddress())
		if err != nil {
			return err
		}
		// nolint:all
		go s.delegatorGrpcServer.Serve(delegatorListener)

		if s.cfg.insecure() {
			// nolint:all
			go s.delegatorHTTPServer.ListenAndServe()
		} else {
			// nolint:all
			go s.delegatorHTTPServer.ListenAndServeTLS("", "")
		}
		log.Infof("started Delegator server at %s", s.cfg.delegatorAddress())
	}

	if s.unlockerSvc != nil {
		if err := s.autoUnlock(); err != nil {
			log.Warnf("failed to auto-unlock: %v", err)
		}
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

	if s.delegatorSvc != nil {
		s.delegatorSvc.Stop()
	}

	s.grpcServer.GracefulStop()
	log.Info("stopped GRPC server")

	// nolint:all
	s.httpServer.Shutdown(context.Background())
	log.Info("stopped HTTP server")

	if s.delegatorGrpcServer != nil {
		s.delegatorGrpcServer.GracefulStop()

		// nolint:all
		s.delegatorConn.Close()

		// nolint:all
		s.delegatorHTTPServer.Shutdown(context.Background())
		log.Info("stopped Delegator server")
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
