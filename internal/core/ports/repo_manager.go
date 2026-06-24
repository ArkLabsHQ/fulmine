package ports

import "github.com/ArkLabsHQ/fulmine/internal/core/domain"

type RepoManager interface {
	Settings() domain.SettingsRepository
	VHTLC() domain.VHTLCRepository
	Delegate() domain.DelegateRepository
	Swap() domain.SwapRepository
	SubscribedScript() domain.SubscribedScriptRepository
	ChainSwaps() domain.ChainSwapRepository
	Close()
}
