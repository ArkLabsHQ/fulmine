package grpc_interface

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"

	pb "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/ark_node/v1"
	"github.com/ArkLabsHQ/ark-node/internal/core/application"
	"github.com/ArkLabsHQ/ark-node/internal/interface/grpc/handlers"
	"github.com/ArkLabsHQ/ark-node/internal/interface/grpc/interceptors"
	"github.com/ArkLabsHQ/ark-node/internal/interface/web"
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
	cfg        Config
	appSvc     *application.Service
	httpServer *http.Server
	grpcServer *grpc.Server

	appStopCh chan struct{}
	feStopCh  chan struct{}
}

func NewService(cfg Config, appSvc *application.Service) (*service, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %s", err)
	}

	appStopCh := make(chan struct{}, 1)
	feStopCh := make(chan struct{}, 1)

	grpcConfig := []grpc.ServerOption{
		interceptors.UnaryInterceptor(),
		interceptors.StreamInterceptor(),
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

	feHandler := web.NewService(appSvc, feStopCh)

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

	return &service{cfg, appSvc, httpServer, grpcServer, appStopCh, feStopCh}, nil
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
