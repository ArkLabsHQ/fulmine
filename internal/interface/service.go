package service_interface

type Service interface {
	Start() error
	Stop()
}
