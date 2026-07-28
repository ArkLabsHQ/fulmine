package ports

import "github.com/ArkLabsHQ/fulmine/internal/core/domain"

type RepoManager interface {
	VHTLC() domain.VHTLCRepository
	Delegate() domain.DelegateRepository
	SubscribedScript() domain.SubscribedScriptRepository
	Close()
}
