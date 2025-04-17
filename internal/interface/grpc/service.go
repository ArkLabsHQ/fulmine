package grpc_interface

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"

	"github.com/ArkLabsHQ/fulmine/internal/core/ports"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/ArkLabsHQ/fulmine/internal/core/application"
	"github.com/ArkLabsHQ/fulmine/internal/interface/grpc/handlers"
	"github.com/ArkLabsHQ/fulmine/internal/interface/grpc/interceptors"
	"github.com/ArkLabsHQ/fulmine/internal/interface/web"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	grpchealth "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/encoding/protojson"
)

type service struct {
	cfg         Config
	appSvc      *application.Service
	httpServer  *http.Server
	grpcServer  *grpc.Server
	unlockerSvc ports.Unlocker

	appStopCh chan struct{}
	feStopCh  chan struct{}
}

func NewService(
	cfg Config, appSvc *application.Service, unlockerSvc ports.Unlocker, sentryEnabled bool,
) (*service, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %s", err)
	}

	appStopCh := make(chan struct{}, 1)
	feStopCh := make(chan struct{}, 1)

	grpcConfig := []grpc.ServerOption{
		interceptors.UnaryInterceptor(sentryEnabled),
		interceptors.StreamInterceptor(sentryEnabled),
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

	healthHandler := handlers.NewHealthHandler()
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

	gwmux := runtime.NewServeMux(
		runtime.WithHealthzEndpoint(grpchealth.NewHealthClient(conn)),
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

	feHandler := web.NewService(appSvc, feStopCh, sentryEnabled)

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

	return &service{
		cfg, appSvc, httpServer, grpcServer, unlockerSvc, feStopCh, appStopCh,
	}, nil
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

	if !s.appSvc.IsReady() {
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
	log.Info("stopped grpc server")
	// nolint:all
	s.httpServer.Shutdown(context.Background())
	log.Info("stopped http server")
}
