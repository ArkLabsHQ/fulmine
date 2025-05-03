package macaroon

import (
	"context"
	"fmt"
	"github.com/ark-network/ark/server/pkg/kvdb"
	log "github.com/sirupsen/logrus"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/ark-network/ark/server/pkg/macaroons"
	"gopkg.in/macaroon-bakery.v2/bakery"
)

const (
	macaroonsLocation = "fulmine"
	macaroonsDbFile   = "macaroons.db"
	macaroonsFolder   = "macaroons"

	userMacaroonFile = "user.macaroon"
)

type Service interface {
	Unlock(ctx context.Context, password string) error
	ChangePassword(ctx context.Context, oldPassword, newPassword string) error
	Generate(ctx context.Context) error
	Auth(ctx context.Context, grpcFullMethodName string) error
}

func NewService(
	datadir string, macFiles, whitelistedMethods, allMethods map[string][]bakery.Op,
) (Service, error) {
	macDatadir := filepath.Join(datadir, macaroonsFolder)
	if err := makeDirectoryIfNotExists(macDatadir); err != nil {
		return nil, err
	}

	macaroonDB, err := kvdb.Create(
		kvdb.BoltBackendName,
		filepath.Join(macDatadir, macaroonsDbFile),
		true,
		kvdb.DefaultDBTimeout,
	)
	if err != nil {
		return nil, err
	}

	keyStore, err := macaroons.NewRootKeyStorage(macaroonDB)
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
		generatedMtx:       &sync.RWMutex{},
		macFiles:           macFiles,
		whitelistedMethods: whitelistedMethods,
		allMethods:         allMethods,
	}, nil
}

type macaroonSvc struct {
	datadir string

	svc *macaroons.Service

	unlocked    bool
	unlockedMtx *sync.RWMutex

	generated    bool
	generatedMtx *sync.RWMutex

	macFiles           map[string][]bakery.Op
	whitelistedMethods map[string][]bakery.Op
	allMethods         map[string][]bakery.Op
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
		return fmt.Errorf("macaroon service is not unlocked")
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
	if m.isGenerated() {
		return nil
	}

	userMacFile := filepath.Join(m.datadir, userMacaroonFile)
	if pathExists(userMacFile) {
		return nil
	}

	for macFilename, macPermissions := range m.macFiles {
		mktMacBytes, err := m.svc.BakeMacaroon(ctx, macPermissions)
		if err != nil {
			return err
		}
		macFile := filepath.Join(m.datadir, macFilename)
		perms := fs.FileMode(0644)
		if err := os.WriteFile(macFile, mktMacBytes, perms); err != nil {
			os.Remove(macFile)
			return err
		}

		m.setGenerated()
	}

	log.Debugf("macaroons generated at %s", m.datadir)

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

func (m *macaroonSvc) isGenerated() bool {
	m.generatedMtx.RLock()
	defer m.generatedMtx.RUnlock()
	return m.generated
}

func (m *macaroonSvc) setGenerated() {
	m.generatedMtx.Lock()
	defer m.generatedMtx.Unlock()
	m.generated = true
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
