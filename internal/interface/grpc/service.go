package grpc_interface

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"

	pb "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/ark_node/v1"
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
	cfg    Config
	server *http.Server
}

func NewService(cfg Config) (*service, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %s", err)
	}

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

	walletHandler := handlers.NewWalletHandler()
	pb.RegisterWalletServiceServer(grpcServer, walletHandler)

	serviceHandler := handlers.NewServiceHandler()
	pb.RegisterServiceServer(grpcServer, serviceHandler)

	notificationHandler := handlers.NewNotificationHandler()
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
	grpcGateway := http.Handler(gwmux)

	feHandler := web.NewService()

	handler := router(grpcServer, grpcGateway)
	mux := http.NewServeMux()
	mux.Handle("/api", handler)
	mux.Handle("/", feHandler)
	mux.Handle("/static/", feHandler)

	httpServerHandler := http.Handler(mux)
	if cfg.insecure() {
		httpServerHandler = h2c.NewHandler(httpServerHandler, &http2.Server{})
	}

	server := &http.Server{
		Addr:      cfg.address(),
		Handler:   httpServerHandler,
		TLSConfig: cfg.tlsConfig(),
	}

	return &service{cfg, server}, nil
}

func (s *service) Start() error {
	if s.cfg.insecure() {
		// nolint:all
		go s.server.ListenAndServe()
	} else {
		// nolint:all
		go s.server.ListenAndServeTLS("", "")
	}
	log.Infof("started listening at %s", s.cfg.address())

	return nil
}

func (s *service) Stop() {
	// nolint:all
	s.server.Shutdown(context.Background())
	log.Info("stopped grpc server")
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
