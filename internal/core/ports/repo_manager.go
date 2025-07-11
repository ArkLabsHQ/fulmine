package ports

import "github.com/ArkLabsHQ/fulmine/internal/core/domain"

type RepoManager interface {
	Settings() domain.SettingsRepository
	VHTLC() domain.VHTLCRepository
	VtxoRollover() domain.VtxoRolloverRepository
	Swap() domain.SwapRepository
	SubscribedScript() domain.SubscribedScriptRepository
	Close()
}
