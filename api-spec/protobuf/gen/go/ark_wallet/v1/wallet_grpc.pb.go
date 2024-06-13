// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.4.0
// - protoc             (unknown)
// source: ark_wallet/v1/wallet.proto

package ark_walletv1

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.62.0 or later.
const _ = grpc.SupportPackageIsVersion8

const (
	WalletService_GenSeed_FullMethodName        = "/ark_wallet.v1.WalletService/GenSeed"
	WalletService_CreateWallet_FullMethodName   = "/ark_wallet.v1.WalletService/CreateWallet"
	WalletService_Unlock_FullMethodName         = "/ark_wallet.v1.WalletService/Unlock"
	WalletService_Lock_FullMethodName           = "/ark_wallet.v1.WalletService/Lock"
	WalletService_ChangePassword_FullMethodName = "/ark_wallet.v1.WalletService/ChangePassword"
	WalletService_RestoreWallet_FullMethodName  = "/ark_wallet.v1.WalletService/RestoreWallet"
	WalletService_Status_FullMethodName         = "/ark_wallet.v1.WalletService/Status"
	WalletService_GetInfo_FullMethodName        = "/ark_wallet.v1.WalletService/GetInfo"
	WalletService_Auth_FullMethodName           = "/ark_wallet.v1.WalletService/Auth"
)

// WalletServiceClient is the client API for WalletService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
//
// WalletService is used to create, or restore an HD Wallet.
// It stores signing seed used for signing of transactions.
// After an HD Wallet is created, the seeds are encrypted and persisted.
// Every time a WalletService is (re)started, it needs to be unlocked with the
// encryption password.
type WalletServiceClient interface {
	// GenSeed returns signing seed that should be used to create a new HD Wallet.
	GenSeed(ctx context.Context, in *GenSeedRequest, opts ...grpc.CallOption) (*GenSeedResponse, error)
	// CreateWallet creates an HD Wallet based on signing seeds,
	// encrypts them with the password and persists the encrypted seeds.
	CreateWallet(ctx context.Context, in *CreateWalletRequest, opts ...grpc.CallOption) (*CreateWalletResponse, error)
	// Unlock tries to unlock the HD Wallet using the given password.
	Unlock(ctx context.Context, in *UnlockRequest, opts ...grpc.CallOption) (*UnlockResponse, error)
	// Lock locks the HD wallet.
	Lock(ctx context.Context, in *LockRequest, opts ...grpc.CallOption) (*LockResponse, error)
	// ChangePassword changes the password used to encrypt/decrypt the HD seeds.
	// It requires the wallet to be locked.
	ChangePassword(ctx context.Context, in *ChangePasswordRequest, opts ...grpc.CallOption) (*ChangePasswordResponse, error)
	// RestoreWallet restores an HD Wallet based on signing seeds,
	// encrypts them with the password and persists the encrypted seeds.
	RestoreWallet(ctx context.Context, in *RestoreWalletRequest, opts ...grpc.CallOption) (WalletService_RestoreWalletClient, error)
	// Status returns info about the status of the wallet.
	Status(ctx context.Context, in *StatusRequest, opts ...grpc.CallOption) (*StatusResponse, error)
	// GetInfo returns info about the HD wallet.
	GetInfo(ctx context.Context, in *GetInfoRequest, opts ...grpc.CallOption) (*GetInfoResponse, error)
	// Auth verifies whether the given password is valid without unlocking the wallet
	Auth(ctx context.Context, in *AuthRequest, opts ...grpc.CallOption) (*AuthResponse, error)
}

type walletServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewWalletServiceClient(cc grpc.ClientConnInterface) WalletServiceClient {
	return &walletServiceClient{cc}
}

func (c *walletServiceClient) GenSeed(ctx context.Context, in *GenSeedRequest, opts ...grpc.CallOption) (*GenSeedResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GenSeedResponse)
	err := c.cc.Invoke(ctx, WalletService_GenSeed_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *walletServiceClient) CreateWallet(ctx context.Context, in *CreateWalletRequest, opts ...grpc.CallOption) (*CreateWalletResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(CreateWalletResponse)
	err := c.cc.Invoke(ctx, WalletService_CreateWallet_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *walletServiceClient) Unlock(ctx context.Context, in *UnlockRequest, opts ...grpc.CallOption) (*UnlockResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(UnlockResponse)
	err := c.cc.Invoke(ctx, WalletService_Unlock_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *walletServiceClient) Lock(ctx context.Context, in *LockRequest, opts ...grpc.CallOption) (*LockResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(LockResponse)
	err := c.cc.Invoke(ctx, WalletService_Lock_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *walletServiceClient) ChangePassword(ctx context.Context, in *ChangePasswordRequest, opts ...grpc.CallOption) (*ChangePasswordResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ChangePasswordResponse)
	err := c.cc.Invoke(ctx, WalletService_ChangePassword_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *walletServiceClient) RestoreWallet(ctx context.Context, in *RestoreWalletRequest, opts ...grpc.CallOption) (WalletService_RestoreWalletClient, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &WalletService_ServiceDesc.Streams[0], WalletService_RestoreWallet_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &walletServiceRestoreWalletClient{ClientStream: stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type WalletService_RestoreWalletClient interface {
	Recv() (*RestoreWalletResponse, error)
	grpc.ClientStream
}

type walletServiceRestoreWalletClient struct {
	grpc.ClientStream
}

func (x *walletServiceRestoreWalletClient) Recv() (*RestoreWalletResponse, error) {
	m := new(RestoreWalletResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *walletServiceClient) Status(ctx context.Context, in *StatusRequest, opts ...grpc.CallOption) (*StatusResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(StatusResponse)
	err := c.cc.Invoke(ctx, WalletService_Status_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *walletServiceClient) GetInfo(ctx context.Context, in *GetInfoRequest, opts ...grpc.CallOption) (*GetInfoResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetInfoResponse)
	err := c.cc.Invoke(ctx, WalletService_GetInfo_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *walletServiceClient) Auth(ctx context.Context, in *AuthRequest, opts ...grpc.CallOption) (*AuthResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(AuthResponse)
	err := c.cc.Invoke(ctx, WalletService_Auth_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// WalletServiceServer is the server API for WalletService service.
// All implementations should embed UnimplementedWalletServiceServer
// for forward compatibility
//
// WalletService is used to create, or restore an HD Wallet.
// It stores signing seed used for signing of transactions.
// After an HD Wallet is created, the seeds are encrypted and persisted.
// Every time a WalletService is (re)started, it needs to be unlocked with the
// encryption password.
type WalletServiceServer interface {
	// GenSeed returns signing seed that should be used to create a new HD Wallet.
	GenSeed(context.Context, *GenSeedRequest) (*GenSeedResponse, error)
	// CreateWallet creates an HD Wallet based on signing seeds,
	// encrypts them with the password and persists the encrypted seeds.
	CreateWallet(context.Context, *CreateWalletRequest) (*CreateWalletResponse, error)
	// Unlock tries to unlock the HD Wallet using the given password.
	Unlock(context.Context, *UnlockRequest) (*UnlockResponse, error)
	// Lock locks the HD wallet.
	Lock(context.Context, *LockRequest) (*LockResponse, error)
	// ChangePassword changes the password used to encrypt/decrypt the HD seeds.
	// It requires the wallet to be locked.
	ChangePassword(context.Context, *ChangePasswordRequest) (*ChangePasswordResponse, error)
	// RestoreWallet restores an HD Wallet based on signing seeds,
	// encrypts them with the password and persists the encrypted seeds.
	RestoreWallet(*RestoreWalletRequest, WalletService_RestoreWalletServer) error
	// Status returns info about the status of the wallet.
	Status(context.Context, *StatusRequest) (*StatusResponse, error)
	// GetInfo returns info about the HD wallet.
	GetInfo(context.Context, *GetInfoRequest) (*GetInfoResponse, error)
	// Auth verifies whether the given password is valid without unlocking the wallet
	Auth(context.Context, *AuthRequest) (*AuthResponse, error)
}

// UnimplementedWalletServiceServer should be embedded to have forward compatible implementations.
type UnimplementedWalletServiceServer struct {
}

func (UnimplementedWalletServiceServer) GenSeed(context.Context, *GenSeedRequest) (*GenSeedResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GenSeed not implemented")
}
func (UnimplementedWalletServiceServer) CreateWallet(context.Context, *CreateWalletRequest) (*CreateWalletResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateWallet not implemented")
}
func (UnimplementedWalletServiceServer) Unlock(context.Context, *UnlockRequest) (*UnlockResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Unlock not implemented")
}
func (UnimplementedWalletServiceServer) Lock(context.Context, *LockRequest) (*LockResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Lock not implemented")
}
func (UnimplementedWalletServiceServer) ChangePassword(context.Context, *ChangePasswordRequest) (*ChangePasswordResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ChangePassword not implemented")
}
func (UnimplementedWalletServiceServer) RestoreWallet(*RestoreWalletRequest, WalletService_RestoreWalletServer) error {
	return status.Errorf(codes.Unimplemented, "method RestoreWallet not implemented")
}
func (UnimplementedWalletServiceServer) Status(context.Context, *StatusRequest) (*StatusResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Status not implemented")
}
func (UnimplementedWalletServiceServer) GetInfo(context.Context, *GetInfoRequest) (*GetInfoResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetInfo not implemented")
}
func (UnimplementedWalletServiceServer) Auth(context.Context, *AuthRequest) (*AuthResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Auth not implemented")
}

// UnsafeWalletServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to WalletServiceServer will
// result in compilation errors.
type UnsafeWalletServiceServer interface {
	mustEmbedUnimplementedWalletServiceServer()
}

func RegisterWalletServiceServer(s grpc.ServiceRegistrar, srv WalletServiceServer) {
	s.RegisterService(&WalletService_ServiceDesc, srv)
}

func _WalletService_GenSeed_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GenSeedRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(WalletServiceServer).GenSeed(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: WalletService_GenSeed_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(WalletServiceServer).GenSeed(ctx, req.(*GenSeedRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _WalletService_CreateWallet_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateWalletRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(WalletServiceServer).CreateWallet(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: WalletService_CreateWallet_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(WalletServiceServer).CreateWallet(ctx, req.(*CreateWalletRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _WalletService_Unlock_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UnlockRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(WalletServiceServer).Unlock(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: WalletService_Unlock_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(WalletServiceServer).Unlock(ctx, req.(*UnlockRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _WalletService_Lock_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(LockRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(WalletServiceServer).Lock(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: WalletService_Lock_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(WalletServiceServer).Lock(ctx, req.(*LockRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _WalletService_ChangePassword_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ChangePasswordRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(WalletServiceServer).ChangePassword(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: WalletService_ChangePassword_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(WalletServiceServer).ChangePassword(ctx, req.(*ChangePasswordRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _WalletService_RestoreWallet_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(RestoreWalletRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(WalletServiceServer).RestoreWallet(m, &walletServiceRestoreWalletServer{ServerStream: stream})
}

type WalletService_RestoreWalletServer interface {
	Send(*RestoreWalletResponse) error
	grpc.ServerStream
}

type walletServiceRestoreWalletServer struct {
	grpc.ServerStream
}

func (x *walletServiceRestoreWalletServer) Send(m *RestoreWalletResponse) error {
	return x.ServerStream.SendMsg(m)
}

func _WalletService_Status_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StatusRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(WalletServiceServer).Status(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: WalletService_Status_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(WalletServiceServer).Status(ctx, req.(*StatusRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _WalletService_GetInfo_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetInfoRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(WalletServiceServer).GetInfo(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: WalletService_GetInfo_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(WalletServiceServer).GetInfo(ctx, req.(*GetInfoRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _WalletService_Auth_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(AuthRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(WalletServiceServer).Auth(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: WalletService_Auth_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(WalletServiceServer).Auth(ctx, req.(*AuthRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// WalletService_ServiceDesc is the grpc.ServiceDesc for WalletService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var WalletService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "ark_wallet.v1.WalletService",
	HandlerType: (*WalletServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GenSeed",
			Handler:    _WalletService_GenSeed_Handler,
		},
		{
			MethodName: "CreateWallet",
			Handler:    _WalletService_CreateWallet_Handler,
		},
		{
			MethodName: "Unlock",
			Handler:    _WalletService_Unlock_Handler,
		},
		{
			MethodName: "Lock",
			Handler:    _WalletService_Lock_Handler,
		},
		{
			MethodName: "ChangePassword",
			Handler:    _WalletService_ChangePassword_Handler,
		},
		{
			MethodName: "Status",
			Handler:    _WalletService_Status_Handler,
		},
		{
			MethodName: "GetInfo",
			Handler:    _WalletService_GetInfo_Handler,
		},
		{
			MethodName: "Auth",
			Handler:    _WalletService_Auth_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "RestoreWallet",
			Handler:       _WalletService_RestoreWallet_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "ark_wallet/v1/wallet.proto",
}
