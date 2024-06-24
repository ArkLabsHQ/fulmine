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
	Service_Address_FullMethodName          = "/ark_wallet.v1.Service/Address"
	Service_Balance_FullMethodName          = "/ark_wallet.v1.Service/Balance"
	Service_Info_FullMethodName             = "/ark_wallet.v1.Service/Info"
	Service_IncreaseInbound_FullMethodName  = "/ark_wallet.v1.Service/IncreaseInbound"
	Service_InboundFees_FullMethodName      = "/ark_wallet.v1.Service/InboundFees"
	Service_IncreaseOutbound_FullMethodName = "/ark_wallet.v1.Service/IncreaseOutbound"
	Service_OutboundFees_FullMethodName     = "/ark_wallet.v1.Service/OutboundFees"
	Service_Offboard_FullMethodName         = "/ark_wallet.v1.Service/Offboard"
	Service_Onboard_FullMethodName          = "/ark_wallet.v1.Service/Onboard"
	Service_Receive_FullMethodName          = "/ark_wallet.v1.Service/Receive"
	Service_ReceiveFees_FullMethodName      = "/ark_wallet.v1.Service/ReceiveFees"
	Service_Send_FullMethodName             = "/ark_wallet.v1.Service/Send"
	Service_SendFees_FullMethodName         = "/ark_wallet.v1.Service/SendFees"
	Service_Round_FullMethodName            = "/ark_wallet.v1.Service/Round"
	Service_Transactions_FullMethodName     = "/ark_wallet.v1.Service/Transactions"
)

// ServiceClient is the client API for Service service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type ServiceClient interface {
	// Address returns offchain address
	Address(ctx context.Context, in *AddressRequest, opts ...grpc.CallOption) (*AddressResponse, error)
	// Balance returns offchain balance
	Balance(ctx context.Context, in *BalanceRequest, opts ...grpc.CallOption) (*BalanceResponse, error)
	// Info returns info about the HD wallet
	Info(ctx context.Context, in *InfoRequest, opts ...grpc.CallOption) (*InfoResponse, error)
	// IncreaseInbound asks to receive funds on requested invoice
	IncreaseInbound(ctx context.Context, in *IncreaseInboundRequest, opts ...grpc.CallOption) (*IncreaseInboundResponse, error)
	// InboundFees returns fees charged to receive requested invoice
	InboundFees(ctx context.Context, in *InboundFeesRequest, opts ...grpc.CallOption) (*InboundFeesResponse, error)
	// IncreaseOutbound requests a ln invoice
	IncreaseOutbound(ctx context.Context, in *IncreaseOutboundRequest, opts ...grpc.CallOption) (*IncreaseOutboundResponse, error)
	// OutboundFees returns fees charged to send amount to ark adddress
	OutboundFees(ctx context.Context, in *OutboundFeesRequest, opts ...grpc.CallOption) (*OutboundFeesResponse, error)
	// Offboard asks to send requested amount to requested onchain address
	Offboard(ctx context.Context, in *OffboardRequest, opts ...grpc.CallOption) (*OffboardResponse, error)
	// Onboard returns onchain address and invoice for requested amount
	Onboard(ctx context.Context, in *OnboardRequest, opts ...grpc.CallOption) (*OnboardResponse, error)
	// Receive returns round info for optional round_id
	Receive(ctx context.Context, in *ReceiveRequest, opts ...grpc.CallOption) (*ReceiveResponse, error)
	// Receive returns round info for optional round_id
	ReceiveFees(ctx context.Context, in *ReceiveFeesRequest, opts ...grpc.CallOption) (*ReceiveFeesResponse, error)
	// Send returns round info for optional round_id
	Send(ctx context.Context, in *SendRequest, opts ...grpc.CallOption) (*SendResponse, error)
	// Send returns round info for optional round_id
	SendFees(ctx context.Context, in *SendFeesRequest, opts ...grpc.CallOption) (*SendFeesResponse, error)
	// Round returns round info for optional round_id
	Round(ctx context.Context, in *RoundRequest, opts ...grpc.CallOption) (*RoundResponse, error)
	// Transactions returns virtual transactions history
	Transactions(ctx context.Context, in *TransactionsRequest, opts ...grpc.CallOption) (*TransactionsResponse, error)
}

type serviceClient struct {
	cc grpc.ClientConnInterface
}

func NewServiceClient(cc grpc.ClientConnInterface) ServiceClient {
	return &serviceClient{cc}
}

func (c *serviceClient) Address(ctx context.Context, in *AddressRequest, opts ...grpc.CallOption) (*AddressResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(AddressResponse)
	err := c.cc.Invoke(ctx, Service_Address_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *serviceClient) Balance(ctx context.Context, in *BalanceRequest, opts ...grpc.CallOption) (*BalanceResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(BalanceResponse)
	err := c.cc.Invoke(ctx, Service_Balance_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *serviceClient) Info(ctx context.Context, in *InfoRequest, opts ...grpc.CallOption) (*InfoResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(InfoResponse)
	err := c.cc.Invoke(ctx, Service_Info_FullMethodName, in, out, cOpts...)
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

func (c *serviceClient) InboundFees(ctx context.Context, in *InboundFeesRequest, opts ...grpc.CallOption) (*InboundFeesResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(InboundFeesResponse)
	err := c.cc.Invoke(ctx, Service_InboundFees_FullMethodName, in, out, cOpts...)
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

func (c *serviceClient) OutboundFees(ctx context.Context, in *OutboundFeesRequest, opts ...grpc.CallOption) (*OutboundFeesResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(OutboundFeesResponse)
	err := c.cc.Invoke(ctx, Service_OutboundFees_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *serviceClient) Offboard(ctx context.Context, in *OffboardRequest, opts ...grpc.CallOption) (*OffboardResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(OffboardResponse)
	err := c.cc.Invoke(ctx, Service_Offboard_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *serviceClient) Onboard(ctx context.Context, in *OnboardRequest, opts ...grpc.CallOption) (*OnboardResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(OnboardResponse)
	err := c.cc.Invoke(ctx, Service_Onboard_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *serviceClient) Receive(ctx context.Context, in *ReceiveRequest, opts ...grpc.CallOption) (*ReceiveResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ReceiveResponse)
	err := c.cc.Invoke(ctx, Service_Receive_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *serviceClient) ReceiveFees(ctx context.Context, in *ReceiveFeesRequest, opts ...grpc.CallOption) (*ReceiveFeesResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ReceiveFeesResponse)
	err := c.cc.Invoke(ctx, Service_ReceiveFees_FullMethodName, in, out, cOpts...)
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

func (c *serviceClient) SendFees(ctx context.Context, in *SendFeesRequest, opts ...grpc.CallOption) (*SendFeesResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(SendFeesResponse)
	err := c.cc.Invoke(ctx, Service_SendFees_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *serviceClient) Round(ctx context.Context, in *RoundRequest, opts ...grpc.CallOption) (*RoundResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(RoundResponse)
	err := c.cc.Invoke(ctx, Service_Round_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *serviceClient) Transactions(ctx context.Context, in *TransactionsRequest, opts ...grpc.CallOption) (*TransactionsResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(TransactionsResponse)
	err := c.cc.Invoke(ctx, Service_Transactions_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ServiceServer is the server API for Service service.
// All implementations should embed UnimplementedServiceServer
// for forward compatibility
type ServiceServer interface {
	// Address returns offchain address
	Address(context.Context, *AddressRequest) (*AddressResponse, error)
	// Balance returns offchain balance
	Balance(context.Context, *BalanceRequest) (*BalanceResponse, error)
	// Info returns info about the HD wallet
	Info(context.Context, *InfoRequest) (*InfoResponse, error)
	// IncreaseInbound asks to receive funds on requested invoice
	IncreaseInbound(context.Context, *IncreaseInboundRequest) (*IncreaseInboundResponse, error)
	// InboundFees returns fees charged to receive requested invoice
	InboundFees(context.Context, *InboundFeesRequest) (*InboundFeesResponse, error)
	// IncreaseOutbound requests a ln invoice
	IncreaseOutbound(context.Context, *IncreaseOutboundRequest) (*IncreaseOutboundResponse, error)
	// OutboundFees returns fees charged to send amount to ark adddress
	OutboundFees(context.Context, *OutboundFeesRequest) (*OutboundFeesResponse, error)
	// Offboard asks to send requested amount to requested onchain address
	Offboard(context.Context, *OffboardRequest) (*OffboardResponse, error)
	// Onboard returns onchain address and invoice for requested amount
	Onboard(context.Context, *OnboardRequest) (*OnboardResponse, error)
	// Receive returns round info for optional round_id
	Receive(context.Context, *ReceiveRequest) (*ReceiveResponse, error)
	// Receive returns round info for optional round_id
	ReceiveFees(context.Context, *ReceiveFeesRequest) (*ReceiveFeesResponse, error)
	// Send returns round info for optional round_id
	Send(context.Context, *SendRequest) (*SendResponse, error)
	// Send returns round info for optional round_id
	SendFees(context.Context, *SendFeesRequest) (*SendFeesResponse, error)
	// Round returns round info for optional round_id
	Round(context.Context, *RoundRequest) (*RoundResponse, error)
	// Transactions returns virtual transactions history
	Transactions(context.Context, *TransactionsRequest) (*TransactionsResponse, error)
}

// UnimplementedServiceServer should be embedded to have forward compatible implementations.
type UnimplementedServiceServer struct {
}

func (UnimplementedServiceServer) Address(context.Context, *AddressRequest) (*AddressResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Address not implemented")
}
func (UnimplementedServiceServer) Balance(context.Context, *BalanceRequest) (*BalanceResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Balance not implemented")
}
func (UnimplementedServiceServer) Info(context.Context, *InfoRequest) (*InfoResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Info not implemented")
}
func (UnimplementedServiceServer) IncreaseInbound(context.Context, *IncreaseInboundRequest) (*IncreaseInboundResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method IncreaseInbound not implemented")
}
func (UnimplementedServiceServer) InboundFees(context.Context, *InboundFeesRequest) (*InboundFeesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method InboundFees not implemented")
}
func (UnimplementedServiceServer) IncreaseOutbound(context.Context, *IncreaseOutboundRequest) (*IncreaseOutboundResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method IncreaseOutbound not implemented")
}
func (UnimplementedServiceServer) OutboundFees(context.Context, *OutboundFeesRequest) (*OutboundFeesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method OutboundFees not implemented")
}
func (UnimplementedServiceServer) Offboard(context.Context, *OffboardRequest) (*OffboardResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Offboard not implemented")
}
func (UnimplementedServiceServer) Onboard(context.Context, *OnboardRequest) (*OnboardResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Onboard not implemented")
}
func (UnimplementedServiceServer) Receive(context.Context, *ReceiveRequest) (*ReceiveResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Receive not implemented")
}
func (UnimplementedServiceServer) ReceiveFees(context.Context, *ReceiveFeesRequest) (*ReceiveFeesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ReceiveFees not implemented")
}
func (UnimplementedServiceServer) Send(context.Context, *SendRequest) (*SendResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Send not implemented")
}
func (UnimplementedServiceServer) SendFees(context.Context, *SendFeesRequest) (*SendFeesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SendFees not implemented")
}
func (UnimplementedServiceServer) Round(context.Context, *RoundRequest) (*RoundResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Round not implemented")
}
func (UnimplementedServiceServer) Transactions(context.Context, *TransactionsRequest) (*TransactionsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Transactions not implemented")
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

func _Service_Address_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(AddressRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).Address(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_Address_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).Address(ctx, req.(*AddressRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Service_Balance_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(BalanceRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).Balance(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_Balance_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).Balance(ctx, req.(*BalanceRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Service_Info_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(InfoRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).Info(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_Info_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).Info(ctx, req.(*InfoRequest))
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

func _Service_InboundFees_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(InboundFeesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).InboundFees(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_InboundFees_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).InboundFees(ctx, req.(*InboundFeesRequest))
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

func _Service_OutboundFees_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(OutboundFeesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).OutboundFees(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_OutboundFees_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).OutboundFees(ctx, req.(*OutboundFeesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Service_Offboard_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(OffboardRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).Offboard(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_Offboard_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).Offboard(ctx, req.(*OffboardRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Service_Onboard_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(OnboardRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).Onboard(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_Onboard_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).Onboard(ctx, req.(*OnboardRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Service_Receive_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ReceiveRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).Receive(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_Receive_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).Receive(ctx, req.(*ReceiveRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Service_ReceiveFees_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ReceiveFeesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).ReceiveFees(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_ReceiveFees_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).ReceiveFees(ctx, req.(*ReceiveFeesRequest))
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

func _Service_SendFees_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SendFeesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).SendFees(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_SendFees_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).SendFees(ctx, req.(*SendFeesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Service_Round_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RoundRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).Round(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_Round_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).Round(ctx, req.(*RoundRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Service_Transactions_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(TransactionsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).Transactions(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Service_Transactions_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).Transactions(ctx, req.(*TransactionsRequest))
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
			MethodName: "Address",
			Handler:    _Service_Address_Handler,
		},
		{
			MethodName: "Balance",
			Handler:    _Service_Balance_Handler,
		},
		{
			MethodName: "Info",
			Handler:    _Service_Info_Handler,
		},
		{
			MethodName: "IncreaseInbound",
			Handler:    _Service_IncreaseInbound_Handler,
		},
		{
			MethodName: "InboundFees",
			Handler:    _Service_InboundFees_Handler,
		},
		{
			MethodName: "IncreaseOutbound",
			Handler:    _Service_IncreaseOutbound_Handler,
		},
		{
			MethodName: "OutboundFees",
			Handler:    _Service_OutboundFees_Handler,
		},
		{
			MethodName: "Offboard",
			Handler:    _Service_Offboard_Handler,
		},
		{
			MethodName: "Onboard",
			Handler:    _Service_Onboard_Handler,
		},
		{
			MethodName: "Receive",
			Handler:    _Service_Receive_Handler,
		},
		{
			MethodName: "ReceiveFees",
			Handler:    _Service_ReceiveFees_Handler,
		},
		{
			MethodName: "Send",
			Handler:    _Service_Send_Handler,
		},
		{
			MethodName: "SendFees",
			Handler:    _Service_SendFees_Handler,
		},
		{
			MethodName: "Round",
			Handler:    _Service_Round_Handler,
		},
		{
			MethodName: "Transactions",
			Handler:    _Service_Transactions_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "ark_wallet/v1/service.proto",
}
