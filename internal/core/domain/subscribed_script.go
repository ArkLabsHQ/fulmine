package domain

import "context"

type SubscribedScriptRepository interface {
	Get(ctx context.Context) ([]string, error)
	Add(ctx context.Context, subscribedScripts []string) (err error)
	Delete(ctx context.Context, subscribedScripts []string) (count int, err error)
}
