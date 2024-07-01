// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.4.0
// - protoc             (unknown)
// source: ark_wallet/v1/service.proto

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
	Service_GetAddress_FullMethodName              = "/ark_wallet.v1.Service/GetAddress"
	Service_GetBalance_FullMethodName              = "/ark_wallet.v1.Service/GetBalance"
	Service_GetInfo_FullMethodName                 = "/ark_wallet.v1.Service/GetInfo"
	Service_IncreaseInbound_FullMethodName         = "/ark_wallet.v1.Service/IncreaseInbound"
	Service_GetIncreaseInboundFees_FullMethodName  = "/ark_wallet.v1.Service/GetIncreaseInboundFees"
	Service_IncreaseOutbound_FullMethodName        = "/ark_wallet.v1.Service/IncreaseOutbound"
	Service_GetIncreaseOutboundFees_FullMethodName = "/ark_wallet.v1.Service/GetIncreaseOutboundFees"
	Service_GetOnboardAddress_FullMethodName       = "/ark_wallet.v1.Service/GetOnboardAddress"
	Service_Send_FullMethodName                    = "/ark_wallet.v1.Service/Send"
	Service_GetSendFees_FullMethodName             = "/ark_wallet.v1.Service/GetSendFees"
	Service_SendOnchain_FullMethodName             = "/ark_wallet.v1.Service/SendOnchain"
	Service_GetSendOnchainFees_FullMethodName      = "/ark_wallet.v1.Service/GetSendOnchainFees"
	Service_GetRoundInfo_FullMethodName            = "/ark_wallet.v1.Service/GetRoundInfo"
	Service_GetTransactions_FullMethodName         = "/ark_wallet.v1.Service/GetTransactions"
)

// ServiceClient is the client API for Service service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type ServiceClient interface {
	// GetAddress returns offchain address
	GetAddress(ctx context.Context, in *GetAddressRequest, opts ...grpc.CallOption) (*GetAddressResponse, error)
	// GetBalance returns ark balance
	GetBalance(ctx context.Context, in *GetBalanceRequest, opts ...grpc.CallOption) (*GetBalanceResponse, error)
	// GetInfo returns info about the ark account
	GetInfo(ctx context.Context, in *GetInfoRequest, opts ...grpc.CallOption) (*GetInfoResponse, error)
	// IncreaseInbound requests a LN invoice to Boltz for me to pay
	IncreaseInbound(ctx context.Context, in *IncreaseInboundRequest, opts ...grpc.CallOption) (*IncreaseInboundResponse, error)
	// GetIncreaseInboundFees returns fees charged to increase inbound liquidity
	GetIncreaseInboundFees(ctx context.Context, in *GetIncreaseInboundFeesRequest, opts ...grpc.CallOption) (*GetIncreaseInboundFeesResponse, error)
	// IncreaseOutbound sends fresh LN invoice to be paid by Boltz
	IncreaseOutbound(ctx context.Context, in *IncreaseOutboundRequest, opts ...grpc.CallOption) (*IncreaseOutboundResponse, error)
	// GetIncreaseOutboundFees returns fees charged to increase outbound liquidity
	GetIncreaseOutboundFees(ctx context.Context, in *GetIncreaseOutboundFeesRequest, opts ...grpc.CallOption) (*GetIncreaseOutboundFeesResponse, error)
	// GetOnboardAddress returns onchain address and invoice for requested amount
	GetOnboardAddress(ctx context.Context, in *GetOnboardAddressRequest, opts ...grpc.CallOption) (*GetOnboardAddressResponse, error)
	// Send asks to send amount to ark address
	Send(ctx context.Context, in *SendRequest, opts ...grpc.CallOption) (*SendResponse, error)
	// GetSendFees returns fees to pay for send request
	GetSendFees(ctx context.Context, in *GetSendFeesRequest, opts ...grpc.CallOption) (*GetSendFeesResponse, error)
	// SendOnchain asks to send requested amount to requested onchain address
	SendOnchain(ctx context.Context, in *SendOnchainRequest, opts ...grpc.CallOption) (*SendOnchainResponse, error)
	// SendOnchain asks to send requested amount to requested onchain address
	GetSendOnchainFees(ctx context.Context, in *GetSendOnchainFeesRequest, opts ...grpc.CallOption) (*GetSendOnchainFeesResponse, error)
	// Round returns round info for optional round_id
	GetRoundInfo(ctx context.Context, in *GetRoundInfoRequest, opts ...grpc.CallOption) (*GetRoundInfoResponse, error)
	// GetTransactions returns virtual transactions history
	GetTransactions(ctx context.Context, in *GetTransactionsRequest, opts ...grpc.CallOption) (*GetTransactionsResponse, error)
}

type serviceClient struct {
	cc grpc.ClientConnInterface
}

func NewServiceClient(cc grpc.ClientConnInterface) ServiceClient {
	return &serviceClient{cc}
}

func (c *serviceClient) GetAddress(ctx context.Context, in *GetAddressRequest, opts ...grpc.CallOption) (*GetAddressResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetAddressResponse)
	err := c.cc.Invoke(ctx, Service_GetAddress_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *serviceClient) GetBalance(ctx context.Context, in *GetBalanceRequest, opts ...grpc.CallOption) (*GetBalanceResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetBalanceResponse)
	err := c.cc.Invoke(ctx, Service_GetBalance_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *serviceClient) GetInfo(ctx context.Context, in *GetInfoRequest, opts ...grpc.CallOption) (*GetInfoResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetInfoResponse)
	err := c.cc.Invoke(ctx, Service_GetInfo_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *serviceClient) IncreaseInbound(ctx context.Context, in *IncreaseInboundRequest, opts ...grpc.CallOption) (*IncreaseInboundResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(IncreaseInboundResponse)
	err := c.cc.Invoke(ctx, Service_IncreaseInbound_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *serviceClient) GetIncreaseInboundFees(ctx context.Context, in *GetIncreaseInboundFeesRequest, opts ...grpc.CallOption) (*GetIncreaseInboundFeesResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetIncreaseInboundFeesResponse)
	err := c.cc.Invoke(ctx, Service_GetIncreaseInboundFees_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *serviceClient) IncreaseOutbound(ctx context.Context, in *IncreaseOutboundRequest, opts ...grpc.CallOption) (*IncreaseOutboundResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(IncreaseOutboundResponse)
	err := c.cc.Invoke(ctx, Service_IncreaseOutbound_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *serviceClient) GetIncreaseOutboundFees(ctx context.Context, in *GetIncreaseOutboundFeesRequest, opts ...grpc.CallOption) (*GetIncreaseOutboundFeesResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetIncreaseOutboundFeesResponse)
	err := c.cc.Invoke(ctx, Service_GetIncreaseOutboundFees_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *serviceClient) GetOnboardAddress(ctx context.Context, in *GetOnboardAddressRequest, opts ...grpc.CallOption) (*GetOnboardAddressResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetOnboardAddressResponse)
	err := c.cc.Invoke(ctx, Service_GetOnboardAddress_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *serviceClient) Send(ctx context.Context, in *SendRequest, opts ...grpc.CallOption) (*SendResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(SendResponse)
	err := c.cc.Invoke(ctx, Service_Send_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *serviceClient) GetSendFees(ctx context.Context, in *GetSendFeesRequest, opts ...grpc.CallOption) (*GetSendFeesResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetSendFeesResponse)
	err := c.cc.Invoke(ctx, Service_GetSendFees_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *serviceClient) SendOnchain(ctx context.Context, in *SendOnchainRequest, opts ...grpc.CallOption) (*SendOnchainResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(SendOnchainResponse)
	err := c.cc.Invoke(ctx, Service_SendOnchain_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *serviceClient) GetSendOnchainFees(ctx context.Context, in *GetSendOnchainFeesRequest, opts ...grpc.CallOption) (*GetSendOnchainFeesResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetSendOnchainFeesResponse)
	err := c.cc.Invoke(ctx, Service_GetSendOnchainFees_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *serviceClient) GetRoundInfo(ctx context.Context, in *GetRoundInfoRequest, opts ...grpc.CallOption) (*GetRoundInfoResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetRoundInfoResponse)
	err := c.cc.Invoke(ctx, Service_GetRoundInfo_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *serviceClient) GetTransactions(ctx context.Context, in *GetTransactionsRequest, opts ...grpc.CallOption) (*GetTransactionsResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetTransactionsResponse)
	err := c.cc.Invoke(ctx, Service_GetTransactions_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ServiceServer is the server API for Service service.
// All implementations should embed UnimplementedServiceServer
// for forward compatibility
type ServiceServer interface {
	// GetAddress returns offchain address
	GetAddress(context.Context, *GetAddressRequest) (*GetAddressResponse, error)
	// GetBalance returns ark balance
	GetBalance(context.Context, *GetBalanceRequest) (*GetBalanceResponse, error)
	// GetInfo returns info about the ark account
	GetInfo(context.Context, *GetInfoRequest) (*GetInfoResponse, error)
	// IncreaseInbound requests a LN invoice to Boltz for me to pay
	IncreaseInbound(context.Context, *IncreaseInboundRequest) (*IncreaseInboundResponse, error)
	// GetIncreaseInboundFees returns fees charged to increase inbound liquidity
	GetIncreaseInboundFees(context.Context, *GetIncreaseInboundFeesRequest) (*GetIncreaseInboundFeesResponse, error)
	// IncreaseOutbound sends fresh LN invoice to be paid by Boltz
	IncreaseOutbound(context.Context, *IncreaseOutboundRequest) (*IncreaseOutboundResponse, error)
	// GetIncreaseOutboundFees returns fees charged to increase outbound liquidity
	GetIncreaseOutboundFees(context.Context, *GetIncreaseOutboundFeesRequest) (*GetIncreaseOutboundFeesResponse, error)
	// GetOnboardAddress returns onchain address and invoice for requested amount
	GetOnboardAddress(context.Context, *GetOnboardAddressRequest) (*GetOnboardAddressResponse, error)
	// Send asks to send amount to ark address
	Send(context.Context, *SendRequest) (*SendResponse, error)
	// GetSendFees returns fees to pay for send request
	GetSendFees(context.Context, *GetSendFeesRequest) (*GetSendFeesResponse, error)
	// SendOnchain asks to send requested amount to requested onchain address
	SendOnchain(context.Context, *SendOnchainRequest) (*SendOnchainResponse, error)
	// SendOnchain asks to send requested amount to requested onchain address
	GetSendOnchainFees(context.Context, *GetSendOnchainFeesRequest) (*GetSendOnchainFeesResponse, error)
	// Round returns round info for optional round_id
	GetRoundInfo(context.Context, *GetRoundInfoRequest) (*GetRoundInfoResponse, error)
	// GetTransactions returns virtual transactions history
	GetTransactions(context.Context, *GetTransactionsRequest) (*GetTransactionsResponse, error)
}

// UnimplementedServiceServer should be embedded to have forward compatible implementations.
type UnimplementedServiceServer struct {
}

func (UnimplementedServiceServer) GetAddress(context.Context, *GetAddressRequest) (*GetAddressResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetAddress not implemented")
}
func (UnimplementedServiceServer) GetBalance(context.Context, *GetBalanceRequest) (*GetBalanceResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetBalance not implemented")
}
func (UnimplementedServiceServer) GetInfo(context.Context, *GetInfoRequest) (*GetInfoResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetInfo not implemented")
}
func (UnimplementedServiceServer) IncreaseInbound(context.Context, *IncreaseInboundRequest) (*IncreaseInboundResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method IncreaseInbound not implemented")
}
func (UnimplementedServiceServer) GetIncreaseInboundFees(context.Context, *GetIncreaseInboundFeesRequest) (*GetIncreaseInboundFeesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetIncreaseInboundFees not implemented")
}
func (UnimplementedServiceServer) IncreaseOutbound(context.Context, *IncreaseOutboundRequest) (*IncreaseOutboundResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method IncreaseOutbound not implemented")
}
func (UnimplementedServiceServer) GetIncreaseOutboundFees(context.Context, *GetIncreaseOutboundFeesRequest) (*GetIncreaseOutboundFeesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetIncreaseOutboundFees not implemented")
}
func (UnimplementedServiceServer) GetOnboardAddress(context.Context, *GetOnboardAddressRequest) (*GetOnboardAddressResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetOnboardAddress not implemented")
}
func (UnimplementedServiceServer) Send(context.Context, *SendRequest) (*SendResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Send not implemented")
}
func (UnimplementedServiceServer) GetSendFees(context.Context, *GetSendFeesRequest) (*GetSendFeesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetSendFees not implemented")
}
func (UnimplementedServiceServer) SendOnchain(context.Context, *SendOnchainRequest) (*SendOnchainResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SendOnchain not implemented")
}
func (UnimplementedServiceServer) GetSendOnchainFees(context.Context, *GetSendOnchainFeesRequest) (*GetSendOnchainFeesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetSendOnchainFees not implemented")
}
func (UnimplementedServiceServer) GetRoundInfo(context.Context, *GetRoundInfoRequest) (*GetRoundInfoResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetRoundInfo not implemented")
}
func (UnimplementedServiceServer) GetTransactions(context.Context, *GetTransactionsRequest) (*GetTransactionsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetTransactions not implemented")
}

// UnsafeServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to ServiceServer will
// result in compilation errors.
type UnsafeServiceServer interface {
	mustEmbedUnimplementedServiceServer()
}

func RegisterServiceServer(s grpc.ServiceRegistrar, srv ServiceServer) {
	s.RegisterService(&Service_ServiceDesc, srv)
}

func _Service_GetAddress_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetAddressRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).GetAddress(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_GetAddress_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).GetAddress(ctx, req.(*GetAddressRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Service_GetBalance_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetBalanceRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).GetBalance(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_GetBalance_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).GetBalance(ctx, req.(*GetBalanceRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Service_GetInfo_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetInfoRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).GetInfo(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_GetInfo_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).GetInfo(ctx, req.(*GetInfoRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Service_IncreaseInbound_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(IncreaseInboundRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).IncreaseInbound(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_IncreaseInbound_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).IncreaseInbound(ctx, req.(*IncreaseInboundRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Service_GetIncreaseInboundFees_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetIncreaseInboundFeesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).GetIncreaseInboundFees(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_GetIncreaseInboundFees_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).GetIncreaseInboundFees(ctx, req.(*GetIncreaseInboundFeesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Service_IncreaseOutbound_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(IncreaseOutboundRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).IncreaseOutbound(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_IncreaseOutbound_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).IncreaseOutbound(ctx, req.(*IncreaseOutboundRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Service_GetIncreaseOutboundFees_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetIncreaseOutboundFeesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).GetIncreaseOutboundFees(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_GetIncreaseOutboundFees_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).GetIncreaseOutboundFees(ctx, req.(*GetIncreaseOutboundFeesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Service_GetOnboardAddress_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetOnboardAddressRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).GetOnboardAddress(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_GetOnboardAddress_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).GetOnboardAddress(ctx, req.(*GetOnboardAddressRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Service_Send_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SendRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).Send(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_Send_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).Send(ctx, req.(*SendRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Service_GetSendFees_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetSendFeesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).GetSendFees(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_GetSendFees_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).GetSendFees(ctx, req.(*GetSendFeesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Service_SendOnchain_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SendOnchainRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).SendOnchain(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_SendOnchain_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).SendOnchain(ctx, req.(*SendOnchainRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Service_GetSendOnchainFees_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetSendOnchainFeesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).GetSendOnchainFees(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_GetSendOnchainFees_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).GetSendOnchainFees(ctx, req.(*GetSendOnchainFeesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Service_GetRoundInfo_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetRoundInfoRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).GetRoundInfo(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_GetRoundInfo_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).GetRoundInfo(ctx, req.(*GetRoundInfoRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Service_GetTransactions_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetTransactionsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).GetTransactions(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_GetTransactions_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).GetTransactions(ctx, req.(*GetTransactionsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Service_ServiceDesc is the grpc.ServiceDesc for Service service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Service_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "ark_wallet.v1.Service",
	HandlerType: (*ServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetAddress",
			Handler:    _Service_GetAddress_Handler,
		},
		{
			MethodName: "GetBalance",
			Handler:    _Service_GetBalance_Handler,
		},
		{
			MethodName: "GetInfo",
			Handler:    _Service_GetInfo_Handler,
		},
		{
			MethodName: "IncreaseInbound",
			Handler:    _Service_IncreaseInbound_Handler,
		},
		{
			MethodName: "GetIncreaseInboundFees",
			Handler:    _Service_GetIncreaseInboundFees_Handler,
		},
		{
			MethodName: "IncreaseOutbound",
			Handler:    _Service_IncreaseOutbound_Handler,
		},
		{
			MethodName: "GetIncreaseOutboundFees",
			Handler:    _Service_GetIncreaseOutboundFees_Handler,
		},
		{
			MethodName: "GetOnboardAddress",
			Handler:    _Service_GetOnboardAddress_Handler,
		},
		{
			MethodName: "Send",
			Handler:    _Service_Send_Handler,
		},
		{
			MethodName: "GetSendFees",
			Handler:    _Service_GetSendFees_Handler,
		},
		{
			MethodName: "SendOnchain",
			Handler:    _Service_SendOnchain_Handler,
		},
		{
			MethodName: "GetSendOnchainFees",
			Handler:    _Service_GetSendOnchainFees_Handler,
		},
		{
			MethodName: "GetRoundInfo",
			Handler:    _Service_GetRoundInfo_Handler,
		},
		{
			MethodName: "GetTransactions",
			Handler:    _Service_GetTransactions_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "ark_wallet/v1/service.proto",
}
