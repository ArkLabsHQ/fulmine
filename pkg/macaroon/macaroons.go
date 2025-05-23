package macaroon

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/ark-network/ark/server/pkg/kvdb"
	"github.com/ark-network/ark/server/pkg/macaroons"
	log "github.com/sirupsen/logrus"
	"gopkg.in/macaroon-bakery.v2/bakery"
)

const (
	macaroonsLocation = "fulmine"
	macaroonsDbFile   = "macaroons.db"
	macaroonsFolder   = "macaroons"
)

type Service interface {
	Unlock(ctx context.Context, password string) error
	ChangePassword(ctx context.Context, oldPassword, newPassword string) error
	Generate(ctx context.Context) error
	Auth(ctx context.Context, grpcFullMethodName string) error
	Reset(ctx context.Context) error
}

type macaroonSvc struct {
	datadir string

	svc *macaroons.Service

	unlocked    bool
	unlockedMtx *sync.RWMutex

	macFiles           map[string][]bakery.Op
	whitelistedMethods map[string][]bakery.Op
	allMethods         map[string][]bakery.Op
}

func NewService(
	datadir string, macFiles, whitelistedMethods, allMethods map[string][]bakery.Op,
) (Service, error) {
	macDatadir := filepath.Join(datadir, macaroonsFolder)
	keyStore, err := initKeyStore(macDatadir)
	if err != nil {
		return nil, err
	}
	svc, err := macaroons.NewService(
		keyStore, macaroonsLocation, false, macaroons.IPLockChecker,
	)
	if err != nil {
		return nil, err
	}

	return &macaroonSvc{
		datadir:            macDatadir,
		svc:                svc,
		unlockedMtx:        &sync.RWMutex{},
		macFiles:           macFiles,
		whitelistedMethods: whitelistedMethods,
		allMethods:         allMethods,
	}, nil
}

func (m *macaroonSvc) Unlock(_ context.Context, password string) error {
	if m.isUnlocked() {
		return nil
	}

	pwd := []byte(password)
	if err := m.svc.CreateUnlock(&pwd); err != nil {
		return err
	}

	m.setUnlocked()

	return nil
}

func (m *macaroonSvc) ChangePassword(_ context.Context, oldPassword, newPassword string) error {
	if !m.isUnlocked() {
		return fmt.Errorf("macaroon service is locked")
	}

	oldPwd := []byte(oldPassword)
	newPwd := []byte(newPassword)
	if err := m.svc.ChangePassword(oldPwd, newPwd); err != nil {
		return err
	}

	m.setUnlocked()

	return nil
}

func (m *macaroonSvc) Generate(ctx context.Context) error {
	generated := false
	for macFilename, macPermissions := range m.macFiles {
		macFile := filepath.Join(m.datadir, macFilename)
		if fileNotExists(macFile) {
			mktMacBytes, err := m.svc.BakeMacaroon(ctx, macPermissions)
			if err != nil {
				return err
			}
			perms := fs.FileMode(0644)
			if err := os.WriteFile(macFile, mktMacBytes, perms); err != nil {
				os.Remove(macFile)
				return err
			}
			generated = true
		}
	}

	if generated {
		log.Debugf("macaroons generated at %s", m.datadir)
	}

	return nil
}

func (m *macaroonSvc) Auth(ctx context.Context, grpcFullMethodName string) error {
	if _, ok := m.whitelistedMethods[grpcFullMethodName]; ok {
		return nil
	}

	uriPermissions, ok := m.allMethods[grpcFullMethodName]
	if !ok {
		return fmt.Errorf("%s: unknown permissions required for method", grpcFullMethodName)
	}

	validator, ok := m.svc.ExternalValidators[grpcFullMethodName]
	if !ok {
		validator = m.svc
	}
	return validator.ValidateMacaroon(ctx, uriPermissions, grpcFullMethodName)
}

func (s *macaroonSvc) Reset(ctx context.Context) error {
	if err := os.RemoveAll(s.datadir); err != nil {
		return err
	}

	keyStore, err := initKeyStore(s.datadir)
	if err != nil {
		return err
	}

	svc, err := macaroons.NewService(
		keyStore, macaroonsLocation, false, macaroons.IPLockChecker,
	)
	if err != nil {
		return err
	}

	s.unlocked = false
	s.svc = svc
	return nil
}

func (m *macaroonSvc) isUnlocked() bool {
	m.unlockedMtx.RLock()
	defer m.unlockedMtx.RUnlock()
	return m.unlocked
}

func (m *macaroonSvc) setUnlocked() {
	m.unlockedMtx.Lock()
	defer m.unlockedMtx.Unlock()
	m.unlocked = true
}

func makeDirectoryIfNotExists(path string) error {
	if pathExists(path) {
		return nil
	}
	return os.MkdirAll(path, os.ModeDir|0755)
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func fileNotExists(path string) bool {
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}

func initKeyStore(datadir string) (*macaroons.RootKeyStorage, error) {
	if err := makeDirectoryIfNotExists(datadir); err != nil {
		return nil, err
	}

	macaroonDB, err := kvdb.Create(
		kvdb.BoltBackendName,
		filepath.Join(datadir, macaroonsDbFile),
		true,
		kvdb.DefaultDBTimeout,
	)
	if err != nil {
		return nil, err
	}

	return macaroons.NewRootKeyStorage(macaroonDB)
}
