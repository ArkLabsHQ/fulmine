package db_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/db"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

var (
	ctx = context.Background()

	testSettings = domain.Settings{
		ApiRoot:     "apiroot",
		ServerUrl:   "serverurl",
		Currency:    "cur",
		EventServer: "eventserver",
		FullNode:    "fullnode",
		Unit:        "unit",
		LnConnectionOpts: &domain.LnConnectionOpts{
			LnDatadir:      "lnd_dir",
			ConnectionType: domain.LND_CONNECTION,
			LnUrl:          "lnd",
		},
	}

	testDelegateTask = func() domain.DelegateTask {
		hash1, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
		input1 := wire.OutPoint{Hash: *hash1, Index: 0}
		return domain.DelegateTask{
			ID:                "test_task_id",
			Intent:            domain.Intent{Message: "test_message", Proof: "test_proof", Txid: "test_txid_1", Inputs: []wire.OutPoint{input1}},
			ForfeitTxs:        map[wire.OutPoint]string{input1: "forfeit_tx_hex"},
			Fee:               1000,
			DelegatorPublicKey: "delegator_pubkey",
			ScheduledAt:       time.Now(),
			Status:            domain.DelegateTaskStatusPending,
			FailReason:        "",
		}
	}()
	secondDelegateTask = func() domain.DelegateTask {
		hash2, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000002")
		input2 := wire.OutPoint{Hash: *hash2, Index: 1}
		return domain.DelegateTask{
			ID:                "second_task_id",
			Intent:            domain.Intent{Message: "second_message", Proof: "second_proof", Txid: "test_txid_2", Inputs: []wire.OutPoint{input2}},
			ForfeitTxs:        map[wire.OutPoint]string{input2: "second_forfeit_tx_hex"},
			Fee:               2000,
			DelegatorPublicKey: "second_delegator_pubkey",
			ScheduledAt:       time.Now().Add(time.Hour),
			Status:            domain.DelegateTaskStatusPending,
			FailReason:        "",
		}
	}()

	testVHTLC = makeVHTLC()

	testSwap   = makeSwap()
	secondSwap = makeSwap()

	testSubscribedScripts = []string{
		"script1",
		"script2",
		"script3",
	}
)

func TestRepoManager(t *testing.T) {
	dbDir := t.TempDir()
	tests := []struct {
		name   string
		config db.ServiceConfig
	}{
		{
			name: "badger",
			config: db.ServiceConfig{
				DbType:   "badger",
				DbConfig: []any{"", nil},
			},
		},
		{
			name: "sqlite",
			config: db.ServiceConfig{
				DbType:   "sqlite",
				DbConfig: []any{dbDir},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := db.NewService(tt.config)
			require.NoError(t, err)
			defer svc.Close()

			testSettingsRepository(t, svc)
			testVHTLCRepository(t, svc)
			testDelegateRepository(t, svc)
			testSwapRepository(t, svc)
			testSubscribedScriptRepository(t, svc)
		})
	}
}

func testSettingsRepository(t *testing.T, svc ports.RepoManager) {
	t.Run("settings repository", func(t *testing.T) {
		testAddSettings(t, svc.Settings())
		testUpdateSettings(t, svc.Settings())
		testCleanSettings(t, svc.Settings())
	})
}

func testVHTLCRepository(t *testing.T, svc ports.RepoManager) {
	t.Run("vHTLC repository", func(t *testing.T) {
		testAddVHTLC(t, svc.VHTLC())
		testGetAllVHTLC(t, svc.VHTLC())
	})
}

func testDelegateRepository(t *testing.T, svc ports.RepoManager) {
	t.Run("delegate repository", func(t *testing.T) {
		testAddDelegateTask(t, svc.Delegate())
		testGetDelegateTaskByID(t, svc.Delegate())
		testGetAllPendingDelegateTasks(t, svc.Delegate())
		testGetPendingTaskByInput(t, svc.Delegate())
		testGetPendingTaskByIntentTxID(t, svc.Delegate())
		testGetAllDelegateTasks(t, svc.Delegate())
		testCancelTasks(t, svc.Delegate())
		testSuccessTasks(t, svc.Delegate())
		testFailTasks(t, svc.Delegate())
	})
}

func testSwapRepository(t *testing.T, svc ports.RepoManager) {
	t.Run("swap repository", func(t *testing.T) {
		testAddSwap(t, svc.Swap())
		testGetAllSwap(t, svc.Swap())
		testUpdateSwap(t, svc.Swap())
	})
}

func testSubscribedScriptRepository(t *testing.T, svc ports.RepoManager) {
	t.Run("subscribed script repository", func(t *testing.T) {
		testAddSubscribedScripts(t, svc.SubscribedScript())
		testDeleteSubscribedScripts(t, svc.SubscribedScript())
	})
}

func testAddSettings(t *testing.T, repo domain.SettingsRepository) {
	t.Run("add settings", func(t *testing.T) {
		settings, err := repo.GetSettings(ctx)
		require.Error(t, err)
		require.Nil(t, settings)

		err = repo.AddSettings(ctx, testSettings)
		require.NoError(t, err)

		settings, err = repo.GetSettings(ctx)
		require.NoError(t, err)
		require.Equal(t, testSettings, *settings)

		err = repo.AddSettings(ctx, testSettings)
		require.Error(t, err)

		err = repo.CleanSettings(ctx)
		require.NoError(t, err)
	})
}

func testUpdateSettings(t *testing.T, repo domain.SettingsRepository) {
	t.Run("update settings", func(t *testing.T) {
		newConnectionOpts := domain.LnConnectionOpts{
			LnDatadir:      "cln_dir",
			ConnectionType: domain.CLN_CONNECTION,
			LnUrl:          "cln",
		}

		newSettings := domain.Settings{
			ApiRoot:          "updated apiroot",
			LnConnectionOpts: &newConnectionOpts,
		}

		err := repo.UpdateSettings(ctx, newSettings)
		require.Error(t, err)

		err = repo.AddSettings(ctx, testSettings)
		require.NoError(t, err)

		expectedSettings := testSettings
		expectedSettings.ApiRoot = newSettings.ApiRoot
		expectedSettings.LnConnectionOpts = &newConnectionOpts

		err = repo.UpdateSettings(ctx, newSettings)
		require.NoError(t, err)

		settings, err := repo.GetSettings(ctx)
		require.NoError(t, err)
		require.NotNil(t, settings)
		require.Equal(t, expectedSettings, *settings)

		newSettings = domain.Settings{
			ServerUrl: "updated serverurl",
			Currency:  "updated cur",
		}
		expectedSettings.ServerUrl = newSettings.ServerUrl
		expectedSettings.Currency = newSettings.Currency

		err = repo.UpdateSettings(ctx, newSettings)
		require.NoError(t, err)
		require.NotNil(t, settings)

		settings, err = repo.GetSettings(ctx)
		require.NoError(t, err)
		require.NotNil(t, settings)
		require.Equal(t, expectedSettings, *settings)
	})
}

func testCleanSettings(t *testing.T, repo domain.SettingsRepository) {
	t.Run("clean settings", func(t *testing.T) {
		settings, err := repo.GetSettings(ctx)
		require.NoError(t, err)
		require.NotNil(t, settings)

		err = repo.CleanSettings(ctx)
		require.NoError(t, err)

		settings, err = repo.GetSettings(ctx)
		require.Error(t, err)
		require.Nil(t, settings)

		err = repo.CleanSettings(ctx)
		require.Error(t, err)
	})
}

func testAddVHTLC(t *testing.T, repo domain.VHTLCRepository) {
	t.Run("add vHTLC", func(t *testing.T) {
		vHTLC, err := repo.Get(ctx, testVHTLC.Id)
		require.Error(t, err)
		require.Nil(t, vHTLC)

		err = repo.Add(ctx, testVHTLC)
		require.NoError(t, err)

		err = repo.Add(ctx, testVHTLC)
		require.Error(t, err)

		vHTLC, err = repo.Get(ctx, testVHTLC.Id)
		require.NoError(t, err)
		require.NotNil(t, vHTLC)
		require.Equal(t, testVHTLC, *vHTLC)

		err = repo.Add(ctx, testVHTLC)
		require.Error(t, err)
	})
}

func testGetAllVHTLC(t *testing.T, repo domain.VHTLCRepository) {
	t.Run("get all vHTLCs", func(t *testing.T) {
		vHTLC, err := repo.GetAll(ctx)
		require.NoError(t, err)
		require.Len(t, vHTLC, 1)

		// Add another vHTLC
		secondVHTLC := makeVHTLC()
		err = repo.Add(ctx, secondVHTLC)
		require.NoError(t, err)

		// Get all vHTLCs
		vhtlcList, err := repo.GetAll(ctx)
		require.NoError(t, err)
		require.Len(t, vhtlcList, 2)
		require.Subset(t, []domain.Vhtlc{testVHTLC, secondVHTLC}, vhtlcList)
	})
}

func testAddDelegateTask(t *testing.T, repo domain.DelegateRepository) {
	t.Run("add delegate task", func(t *testing.T) {
		task, err := repo.GetByID(ctx, testDelegateTask.ID)
		require.Error(t, err)
		require.Nil(t, task)

		err = repo.Add(ctx, testDelegateTask)
		require.NoError(t, err)

		task, err = repo.GetByID(ctx, testDelegateTask.ID)
		require.NoError(t, err)
		require.NotNil(t, task)
		require.Equal(t, testDelegateTask.ID, task.ID)
		require.Equal(t, testDelegateTask.Intent, task.Intent)
		require.Equal(t, testDelegateTask.Status, task.Status)
		require.Equal(t, testDelegateTask.Fee, task.Fee)
		require.Equal(t, testDelegateTask.DelegatorPublicKey, task.DelegatorPublicKey)
		require.Equal(t, testDelegateTask.Intent.Inputs, task.Intent.Inputs)
		require.Equal(t, testDelegateTask.ForfeitTxs, task.ForfeitTxs)
	})

	t.Run("add delegate task with multiple inputs", func(t *testing.T) {
		hash1, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000010")
		hash2, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000011")
		input1 := wire.OutPoint{Hash: *hash1, Index: 0}
		input2 := wire.OutPoint{Hash: *hash2, Index: 1}

		multiInputTask := domain.DelegateTask{
			ID:                "multi_input_task",
			Intent:            domain.Intent{Message: "multi_input_message", Proof: "multi_input_proof", Txid: "multi_input_txid", Inputs: []wire.OutPoint{input1, input2}},
			ForfeitTxs:        map[wire.OutPoint]string{input1: "forfeit_tx_1", input2: "forfeit_tx_2"},
			Fee:               3000,
			DelegatorPublicKey: "multi_input_pubkey",
			ScheduledAt:       time.Now(),
			Status:            domain.DelegateTaskStatusPending,
		}

		err := repo.Add(ctx, multiInputTask)
		require.NoError(t, err)

		task, err := repo.GetByID(ctx, multiInputTask.ID)
		require.NoError(t, err)
		require.NotNil(t, task)
		require.Len(t, task.Intent.Inputs, 2)
		require.Len(t, task.ForfeitTxs, 2)
		require.Equal(t, multiInputTask.Intent.Inputs, task.Intent.Inputs)
		require.Equal(t, multiInputTask.ForfeitTxs, task.ForfeitTxs)
	})

	t.Run("add delegate task with no inputs", func(t *testing.T) {
		noInputTask := domain.DelegateTask{
			ID:                "no_input_task",
			Intent:            domain.Intent{Message: "no_input_message", Proof: "no_input_proof", Txid: "no_input_txid", Inputs: []wire.OutPoint{}},
			ForfeitTxs:        map[wire.OutPoint]string{},
			Fee:               4000,
			DelegatorPublicKey: "no_input_pubkey",
			ScheduledAt:       time.Now(),
			Status:            domain.DelegateTaskStatusPending,
		}

		err := repo.Add(ctx, noInputTask)
		require.NoError(t, err)

		task, err := repo.GetByID(ctx, noInputTask.ID)
		require.NoError(t, err)
		require.NotNil(t, task)
		require.Len(t, task.Intent.Inputs, 0)
		require.Len(t, task.ForfeitTxs, 0)
	})

	t.Run("add delegate task with some inputs having forfeit txs", func(t *testing.T) {
		hash1, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000020")
		hash2, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000021")
		input1 := wire.OutPoint{Hash: *hash1, Index: 0}
		input2 := wire.OutPoint{Hash: *hash2, Index: 1}

		partialForfeitTask := domain.DelegateTask{
			ID:                "partial_forfeit_task",
			Intent:            domain.Intent{Message: "partial_forfeit_message", Proof: "partial_forfeit_proof", Txid: "partial_forfeit_txid", Inputs: []wire.OutPoint{input1, input2}},
			ForfeitTxs:        map[wire.OutPoint]string{input1: "forfeit_tx_only_for_input1"},
			Fee:               5000,
			DelegatorPublicKey: "partial_forfeit_pubkey",
			ScheduledAt:       time.Now(),
			Status:            domain.DelegateTaskStatusPending,
		}

		err := repo.Add(ctx, partialForfeitTask)
		require.NoError(t, err)

		task, err := repo.GetByID(ctx, partialForfeitTask.ID)
		require.NoError(t, err)
		require.NotNil(t, task)
		require.Len(t, task.Intent.Inputs, 2)
		require.Len(t, task.ForfeitTxs, 1)
		require.Equal(t, "forfeit_tx_only_for_input1", task.ForfeitTxs[input1])
		_, exists := task.ForfeitTxs[input2]
		require.False(t, exists)
	})
}

func testGetDelegateTaskByID(t *testing.T, repo domain.DelegateRepository) {
	t.Run("get delegate task by id", func(t *testing.T) {
		// Reset task to pending for this test
		testTask := testDelegateTask
		testTask.ID = "get_by_id_task"
		testTask.Status = domain.DelegateTaskStatusPending
		err := repo.Add(ctx, testTask)
		require.NoError(t, err)

		task, err := repo.GetByID(ctx, testTask.ID)
		require.NoError(t, err)
		require.NotNil(t, task)
		require.Equal(t, testTask.ID, task.ID)
		require.Equal(t, testTask.Intent, task.Intent)
		require.Equal(t, testTask.Fee, task.Fee)
		require.Equal(t, testTask.DelegatorPublicKey, task.DelegatorPublicKey)
		require.Equal(t, testTask.Intent.Inputs, task.Intent.Inputs)
		require.Equal(t, testTask.ForfeitTxs, task.ForfeitTxs)

		_, err = repo.GetByID(ctx, "non_existent_id")
		require.Error(t, err)
	})

	t.Run("get delegate task with multiple inputs", func(t *testing.T) {
		hash1, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000030")
		hash2, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000031")
		hash3, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000032")
		input1 := wire.OutPoint{Hash: *hash1, Index: 0}
		input2 := wire.OutPoint{Hash: *hash2, Index: 1}
		input3 := wire.OutPoint{Hash: *hash3, Index: 2}

		multiInputTask := domain.DelegateTask{
			ID:                "get_multi_input_task",
			Intent:            domain.Intent{Message: "get_multi_input", Proof: "proof", Txid: "get_multi_input_txid", Inputs: []wire.OutPoint{input1, input2, input3}},
			ForfeitTxs:        map[wire.OutPoint]string{input1: "ft1", input2: "ft2", input3: "ft3"},
			Fee:               6000,
			DelegatorPublicKey: "get_multi_pubkey",
			ScheduledAt:       time.Now(),
			Status:            domain.DelegateTaskStatusPending,
		}

		err := repo.Add(ctx, multiInputTask)
		require.NoError(t, err)

		task, err := repo.GetByID(ctx, multiInputTask.ID)
		require.NoError(t, err)
		require.NotNil(t, task)
		require.Len(t, task.Intent.Inputs, 3)
		require.Len(t, task.ForfeitTxs, 3)
		require.Contains(t, task.Intent.Inputs, input1)
		require.Contains(t, task.Intent.Inputs, input2)
		require.Contains(t, task.Intent.Inputs, input3)
		require.Equal(t, "ft1", task.ForfeitTxs[input1])
		require.Equal(t, "ft2", task.ForfeitTxs[input2])
		require.Equal(t, "ft3", task.ForfeitTxs[input3])
	})
}

func testGetAllPendingDelegateTasks(t *testing.T, repo domain.DelegateRepository) {
	t.Run("get all pending delegate tasks", func(t *testing.T) {
		pendingTask := testDelegateTask
		pendingTask.ID = "pending_task_1"
		pendingTask.Status = domain.DelegateTaskStatusPending
		err := repo.Add(ctx, pendingTask)
		require.NoError(t, err)

		doneTask := secondDelegateTask
		doneTask.ID = "done_task_1"
		doneTask.Status = domain.DelegateTaskStatusPending
		err = repo.Add(ctx, doneTask)
		require.NoError(t, err)
		err = repo.CompleteTasks(ctx, "commitment_txid_1", doneTask.ID)
		require.NoError(t, err)

		anotherPendingTask := secondDelegateTask
		anotherPendingTask.ID = "pending_task_2"
		anotherPendingTask.Status = domain.DelegateTaskStatusPending
		err = repo.Add(ctx, anotherPendingTask)
		require.NoError(t, err)

		pendingTasks, err := repo.GetAllPending(ctx)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(pendingTasks), 2)

		taskIDs := make(map[string]bool)
		for _, pendingTask := range pendingTasks {
			taskIDs[pendingTask.ID] = true
			fullTask, err := repo.GetByID(ctx, pendingTask.ID)
			require.NoError(t, err)
			require.Equal(t, domain.DelegateTaskStatusPending, fullTask.Status)
			require.Equal(t, pendingTask.ScheduledAt.Unix(), fullTask.ScheduledAt.Unix())
		}
		require.True(t, taskIDs["pending_task_1"])
		require.True(t, taskIDs["pending_task_2"])
		require.False(t, taskIDs["done_task_1"])
	})
}

func testGetPendingTaskByInput(t *testing.T, repo domain.DelegateRepository) {
	t.Run("get pending task by input", func(t *testing.T) {
		hash1, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000003")
		testInput := wire.OutPoint{Hash: *hash1, Index: 0}

		pendingTask1 := testDelegateTask
		pendingTask1.ID = "pending_by_input_1"
		pendingTask1.Intent.Inputs = []wire.OutPoint{testInput}
		pendingTask1.Status = domain.DelegateTaskStatusPending
		err := repo.Add(ctx, pendingTask1)
		require.NoError(t, err)

		pendingTask2 := testDelegateTask
		pendingTask2.ID = "pending_by_input_2"
		pendingTask2.Intent.Inputs = []wire.OutPoint{testInput}
		pendingTask2.Status = domain.DelegateTaskStatusPending
		err = repo.Add(ctx, pendingTask2)
		require.NoError(t, err)

		hash2, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000004")
		differentInput := wire.OutPoint{Hash: *hash2, Index: 0}
		pendingTask3 := testDelegateTask
		pendingTask3.ID = "pending_by_input_3"
		pendingTask3.Intent.Inputs = []wire.OutPoint{differentInput}
		pendingTask3.Status = domain.DelegateTaskStatusPending
		err = repo.Add(ctx, pendingTask3)
		require.NoError(t, err)

		doneTask := testDelegateTask
		doneTask.ID = "done_by_input_1"
		doneTask.Intent.Inputs = []wire.OutPoint{testInput}
		doneTask.Status = domain.DelegateTaskStatusPending
		err = repo.Add(ctx, doneTask)
		require.NoError(t, err)
		err = repo.CompleteTasks(ctx, "commitment_txid_1", doneTask.ID)
		require.NoError(t, err)

		taskIDs, err := repo.GetPendingTaskIDsByInputs(ctx, []wire.OutPoint{testInput})
		require.NoError(t, err)
		require.Len(t, taskIDs, 2)

		taskIDSet := make(map[string]bool)
		for _, id := range taskIDs {
			taskIDSet[id] = true
		}
		require.True(t, taskIDSet["pending_by_input_1"])
		require.True(t, taskIDSet["pending_by_input_2"])
		require.False(t, taskIDSet["pending_by_input_3"])
		require.False(t, taskIDSet["done_by_input_1"])

		hash3, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000005")
		nonExistentInput := wire.OutPoint{Hash: *hash3, Index: 0}
		taskIDs, err = repo.GetPendingTaskIDsByInputs(ctx, []wire.OutPoint{nonExistentInput})
		require.NoError(t, err)
		require.Len(t, taskIDs, 0)

		hash4, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000006")
		anotherInput := wire.OutPoint{Hash: *hash4, Index: 0}
		pendingTask4 := testDelegateTask
		pendingTask4.ID = "pending_by_input_4"
		pendingTask4.Intent.Inputs = []wire.OutPoint{anotherInput}
		pendingTask4.Status = domain.DelegateTaskStatusPending
		err = repo.Add(ctx, pendingTask4)
		require.NoError(t, err)

		taskIDs, err = repo.GetPendingTaskIDsByInputs(ctx, []wire.OutPoint{testInput, anotherInput})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(taskIDs), 3)
	})

	t.Run("get pending task by input with empty list", func(t *testing.T) {
		taskIDs, err := repo.GetPendingTaskIDsByInputs(ctx, []wire.OutPoint{})
		require.NoError(t, err)
		require.Len(t, taskIDs, 0)
	})
}

func testGetPendingTaskByIntentTxID(t *testing.T, repo domain.DelegateRepository) {
	t.Run("get pending task by intent txid", func(t *testing.T) {
		testIntentTxid := "test_intent_txid_123"

		pendingTask1 := testDelegateTask
		pendingTask1.ID = "pending_by_txid_1"
		pendingTask1.Intent.Txid = testIntentTxid
		pendingTask1.Status = domain.DelegateTaskStatusPending
		err := repo.Add(ctx, pendingTask1)
		require.NoError(t, err)

		task, err := repo.GetPendingTaskByIntentTxID(ctx, testIntentTxid)
		require.NoError(t, err)
		require.NotNil(t, task)
		require.Equal(t, pendingTask1.ID, task.ID)
		require.Equal(t, pendingTask1.ScheduledAt.Unix(), task.ScheduledAt.Unix())

		anotherIntentTxid := "test_intent_txid_456"
		pendingTask2 := testDelegateTask
		pendingTask2.ID = "pending_by_txid_2"
		pendingTask2.Intent.Txid = anotherIntentTxid
		pendingTask2.Status = domain.DelegateTaskStatusPending
		err = repo.Add(ctx, pendingTask2)
		require.NoError(t, err)

		task, err = repo.GetPendingTaskByIntentTxID(ctx, anotherIntentTxid)
		require.NoError(t, err)
		require.NotNil(t, task)
		require.Equal(t, pendingTask2.ID, task.ID)

		doneTask := testDelegateTask
		doneTask.ID = "done_by_txid_1"
		doneTask.Intent.Txid = testIntentTxid
		doneTask.Status = domain.DelegateTaskStatusPending
		err = repo.Add(ctx, doneTask)
		require.NoError(t, err)
		err = repo.CompleteTasks(ctx, "commitment_txid_1", doneTask.ID)
		require.NoError(t, err)

		task, err = repo.GetPendingTaskByIntentTxID(ctx, testIntentTxid)
		require.NoError(t, err)
		require.NotNil(t, task)
		require.Equal(t, pendingTask1.ID, task.ID)

		_, err = repo.GetPendingTaskByIntentTxID(ctx, "non_existent_txid")
		require.Error(t, err)
		require.Contains(t, err.Error(), "not found")
	})

	t.Run("get pending task by intent txid with multiple pending tasks", func(t *testing.T) {
		sharedIntentTxid := "shared_intent_txid_789"

		pendingTask1 := testDelegateTask
		pendingTask1.ID = "shared_pending_1"
		pendingTask1.Intent.Txid = sharedIntentTxid
		pendingTask1.Status = domain.DelegateTaskStatusPending
		err := repo.Add(ctx, pendingTask1)
		require.NoError(t, err)

		pendingTask2 := testDelegateTask
		pendingTask2.ID = "shared_pending_2"
		pendingTask2.Intent.Txid = sharedIntentTxid
		pendingTask2.Status = domain.DelegateTaskStatusPending
		err = repo.Add(ctx, pendingTask2)
		require.NoError(t, err)

		task, err := repo.GetPendingTaskByIntentTxID(ctx, sharedIntentTxid)
		require.NoError(t, err)
		require.NotNil(t, task)
		require.True(t, task.ID == "shared_pending_1" || task.ID == "shared_pending_2")
	})
}

func testGetAllDelegateTasks(t *testing.T, repo domain.DelegateRepository) {
	t.Run("get all delegate tasks", func(t *testing.T) {
		pendingTask1 := testDelegateTask
		pendingTask1.ID = "getall_pending_1"
		pendingTask1.Status = domain.DelegateTaskStatusPending
		err := repo.Add(ctx, pendingTask1)
		require.NoError(t, err)

		pendingTask2 := testDelegateTask
		pendingTask2.ID = "getall_pending_2"
		pendingTask2.Status = domain.DelegateTaskStatusPending
		err = repo.Add(ctx, pendingTask2)
		require.NoError(t, err)

		completedTask1 := testDelegateTask
		completedTask1.ID = "getall_completed_1"
		completedTask1.Status = domain.DelegateTaskStatusPending
		err = repo.Add(ctx, completedTask1)
		require.NoError(t, err)
		err = repo.CompleteTasks(ctx, "commitment_txid_getall_1", completedTask1.ID)
		require.NoError(t, err)

		completedTask2 := testDelegateTask
		completedTask2.ID = "getall_completed_2"
		completedTask2.Status = domain.DelegateTaskStatusPending
		err = repo.Add(ctx, completedTask2)
		require.NoError(t, err)
		err = repo.CompleteTasks(ctx, "commitment_txid_getall_2", completedTask2.ID)
		require.NoError(t, err)

		failedTask1 := testDelegateTask
		failedTask1.ID = "getall_failed_1"
		failedTask1.Status = domain.DelegateTaskStatusPending
		err = repo.Add(ctx, failedTask1)
		require.NoError(t, err)
		err = repo.FailTasks(ctx, "test failure reason", failedTask1.ID)
		require.NoError(t, err)

		pendingTasks, err := repo.GetAll(ctx, domain.DelegateTaskStatusPending, 100, 0)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(pendingTasks), 2)

		taskIDs := make(map[string]bool)
		for _, task := range pendingTasks {
			taskIDs[task.ID] = true
			require.Equal(t, domain.DelegateTaskStatusPending, task.Status)
		}
		require.True(t, taskIDs["getall_pending_1"])
		require.True(t, taskIDs["getall_pending_2"])
		require.False(t, taskIDs["getall_completed_1"])
		require.False(t, taskIDs["getall_completed_2"])
		require.False(t, taskIDs["getall_failed_1"])

		completedTasks, err := repo.GetAll(ctx, domain.DelegateTaskStatusCompleted, 100, 0)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(completedTasks), 2)

		taskIDs = make(map[string]bool)
		for _, task := range completedTasks {
			taskIDs[task.ID] = true
			require.Equal(t, domain.DelegateTaskStatusCompleted, task.Status)
		}
		require.True(t, taskIDs["getall_completed_1"])
		require.True(t, taskIDs["getall_completed_2"])
		require.False(t, taskIDs["getall_pending_1"])
		require.False(t, taskIDs["getall_pending_2"])
		require.False(t, taskIDs["getall_failed_1"])

		failedTasks, err := repo.GetAll(ctx, domain.DelegateTaskStatusFailed, 100, 0)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(failedTasks), 1)

		taskIDs = make(map[string]bool)
		for _, task := range failedTasks {
			taskIDs[task.ID] = true
			require.Equal(t, domain.DelegateTaskStatusFailed, task.Status)
			if task.ID == "getall_failed_1" {
				require.Equal(t, "test failure reason", task.FailReason)
			}
		}
		require.True(t, taskIDs["getall_failed_1"])
		require.False(t, taskIDs["getall_pending_1"])
		require.False(t, taskIDs["getall_completed_1"])

		cancelledTask1 := testDelegateTask
		cancelledTask1.ID = "getall_cancelled_1"
		cancelledTask1.Status = domain.DelegateTaskStatusPending
		err = repo.Add(ctx, cancelledTask1)
		require.NoError(t, err)
		err = repo.CancelTasks(ctx, cancelledTask1.ID)
		require.NoError(t, err)

		cancelledTasks, err := repo.GetAll(ctx, domain.DelegateTaskStatusCancelled, 100, 0)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(cancelledTasks), 1)

		taskIDs = make(map[string]bool)
		for _, task := range cancelledTasks {
			taskIDs[task.ID] = true
			require.Equal(t, domain.DelegateTaskStatusCancelled, task.Status)
		}
		require.True(t, taskIDs["getall_cancelled_1"])
	})

	t.Run("get all delegate tasks with limit and offset", func(t *testing.T) {
		initialTasks, err := repo.GetAll(ctx, domain.DelegateTaskStatusPending, 1000, 0)
		require.NoError(t, err)
		initialCount := len(initialTasks)

		testTaskIDs := make([]string, 5)
		for i := range 5 {
			task := testDelegateTask
			task.ID = fmt.Sprintf("getall_limit_%d", i)
			task.Status = domain.DelegateTaskStatusPending
			task.ScheduledAt = time.Now().Add(time.Duration(i) * time.Second)
			err := repo.Add(ctx, task)
			require.NoError(t, err)
			testTaskIDs[i] = task.ID
		}

		tasks, err := repo.GetAll(ctx, domain.DelegateTaskStatusPending, 2, 0)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(tasks), 2)

		firstPageIDs := make(map[string]bool)
		for _, task := range tasks {
			firstPageIDs[task.ID] = true
		}
		hasTestTask := false
		for _, testID := range testTaskIDs {
			if firstPageIDs[testID] {
				hasTestTask = true
				break
			}
		}
		require.True(t, hasTestTask, "At least one test task should be in the first page")

		tasks, err = repo.GetAll(ctx, domain.DelegateTaskStatusPending, 2, 2)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(tasks), 1)

		allTasks, err := repo.GetAll(ctx, domain.DelegateTaskStatusPending, 1000, 0)
		require.NoError(t, err)
		require.Equal(t, initialCount+5, len(allTasks))

		allTaskIDs := make(map[string]bool)
		for _, task := range allTasks {
			allTaskIDs[task.ID] = true
		}
		for _, testID := range testTaskIDs {
			require.True(t, allTaskIDs[testID], "Test task %s should be in the full list", testID)
		}
	})

	t.Run("get all delegate tasks with multiple inputs", func(t *testing.T) {
		hash1, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000100")
		hash2, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000101")
		input1 := wire.OutPoint{Hash: *hash1, Index: 0}
		input2 := wire.OutPoint{Hash: *hash2, Index: 1}

		multiInputTask := domain.DelegateTask{
			ID:                "getall_multi_input",
			Intent:            domain.Intent{Message: "getall_multi", Proof: "proof", Txid: "getall_multi_txid", Inputs: []wire.OutPoint{input1, input2}},
			ForfeitTxs:        map[wire.OutPoint]string{input1: "ft1", input2: "ft2"},
			Fee:               7000,
			DelegatorPublicKey: "getall_multi_pubkey",
			ScheduledAt:       time.Now(),
			Status:            domain.DelegateTaskStatusPending,
		}

		err := repo.Add(ctx, multiInputTask)
		require.NoError(t, err)

		tasks, err := repo.GetAll(ctx, domain.DelegateTaskStatusPending, 100, 0)
		require.NoError(t, err)

		var foundTask *domain.DelegateTask
		for i := range tasks {
			if tasks[i].ID == "getall_multi_input" {
				foundTask = &tasks[i]
				break
			}
		}
		require.NotNil(t, foundTask)
		require.Len(t, foundTask.Intent.Inputs, 2)
		require.Len(t, foundTask.ForfeitTxs, 2)
		require.Contains(t, foundTask.Intent.Inputs, input1)
		require.Contains(t, foundTask.Intent.Inputs, input2)
		require.Equal(t, "ft1", foundTask.ForfeitTxs[input1])
		require.Equal(t, "ft2", foundTask.ForfeitTxs[input2])
	})

	t.Run("get all delegate tasks with empty result", func(t *testing.T) {
		tasks, err := repo.GetAll(ctx, domain.DelegateTaskStatusPending, 100, 10000)
		require.NoError(t, err)
		require.Len(t, tasks, 0)
	})

	t.Run("get all delegate tasks ordered by scheduled_at desc", func(t *testing.T) {
		baseTime := time.Now()
		
		task1 := testDelegateTask
		task1.ID = "order_test_1"
		task1.Status = domain.DelegateTaskStatusPending
		task1.ScheduledAt = baseTime.Add(3 * time.Second)
		err := repo.Add(ctx, task1)
		require.NoError(t, err)

		task2 := testDelegateTask
		task2.ID = "order_test_2"
		task2.Status = domain.DelegateTaskStatusPending
		task2.ScheduledAt = baseTime.Add(1 * time.Second)
		err = repo.Add(ctx, task2)
		require.NoError(t, err)

		task3 := testDelegateTask
		task3.ID = "order_test_3"
		task3.Status = domain.DelegateTaskStatusPending
		task3.ScheduledAt = baseTime.Add(5 * time.Second)
		err = repo.Add(ctx, task3)
		require.NoError(t, err)

		task4 := testDelegateTask
		task4.ID = "order_test_4"
		task4.Status = domain.DelegateTaskStatusPending
		task4.ScheduledAt = baseTime.Add(2 * time.Second)
		err = repo.Add(ctx, task4)
		require.NoError(t, err)

		tasks, err := repo.GetAll(ctx, domain.DelegateTaskStatusPending, 1000, 0)
		require.NoError(t, err)

		require.GreaterOrEqual(t, len(tasks), 4, "Should have at least 4 tasks")

		for i := 0; i < len(tasks)-1; i++ {
			require.GreaterOrEqual(t, tasks[i].ScheduledAt.Unix(), tasks[i+1].ScheduledAt.Unix(),
				"All tasks should be ordered by ScheduledAt DESC (most recent first). Task at index %d (ID: %s, scheduled at %v) should come before task at index %d (ID: %s, scheduled at %v)",
				i, tasks[i].ID, tasks[i].ScheduledAt, i+1, tasks[i+1].ID, tasks[i+1].ScheduledAt)
		}

		testTaskMap := make(map[string]domain.DelegateTask)
		for _, task := range tasks {
			if task.ID == "order_test_1" || task.ID == "order_test_2" || 
			   task.ID == "order_test_3" || task.ID == "order_test_4" {
				testTaskMap[task.ID] = task
			}
		}

		require.Len(t, testTaskMap, 4, "Should find exactly 4 test tasks")
		require.Equal(t, baseTime.Add(5*time.Second).Unix(), testTaskMap["order_test_3"].ScheduledAt.Unix(), "order_test_3 should have the most recent ScheduledAt")
		require.Equal(t, baseTime.Add(3*time.Second).Unix(), testTaskMap["order_test_1"].ScheduledAt.Unix(), "order_test_1 should have the second most recent ScheduledAt")
		require.Equal(t, baseTime.Add(2*time.Second).Unix(), testTaskMap["order_test_4"].ScheduledAt.Unix(), "order_test_4 should have the third most recent ScheduledAt")
		require.Equal(t, baseTime.Add(1*time.Second).Unix(), testTaskMap["order_test_2"].ScheduledAt.Unix(), "order_test_2 should have the oldest ScheduledAt")
	})
}

func testCancelTasks(t *testing.T, repo domain.DelegateRepository) {
	t.Run("cancel tasks", func(t *testing.T) {
		cancelTask1 := testDelegateTask
		cancelTask1.ID = "cancel_task_1"
		cancelTask1.Status = domain.DelegateTaskStatusPending
		err := repo.Add(ctx, cancelTask1)
		require.NoError(t, err)

		cancelTask2 := testDelegateTask
		cancelTask2.ID = "cancel_task_2"
		cancelTask2.Status = domain.DelegateTaskStatusPending
		err = repo.Add(ctx, cancelTask2)
		require.NoError(t, err)

		task1, err := repo.GetByID(ctx, cancelTask1.ID)
		require.NoError(t, err)
		require.Equal(t, domain.DelegateTaskStatusPending, task1.Status)

		task2, err := repo.GetByID(ctx, cancelTask2.ID)
		require.NoError(t, err)
		require.Equal(t, domain.DelegateTaskStatusPending, task2.Status)

		err = repo.CancelTasks(ctx, cancelTask1.ID, cancelTask2.ID)
		require.NoError(t, err)

		task1, err = repo.GetByID(ctx, cancelTask1.ID)
		require.NoError(t, err)
		require.Equal(t, domain.DelegateTaskStatusCancelled, task1.Status)

		task2, err = repo.GetByID(ctx, cancelTask2.ID)
		require.NoError(t, err)
		require.Equal(t, domain.DelegateTaskStatusCancelled, task2.Status)

		pendingTasks, err := repo.GetAllPending(ctx)
		require.NoError(t, err)
		for _, pendingTask := range pendingTasks {
			require.NotEqual(t, cancelTask1.ID, pendingTask.ID)
			require.NotEqual(t, cancelTask2.ID, pendingTask.ID)
		}
	})

	t.Run("cancel tasks with empty list", func(t *testing.T) {
		err := repo.CancelTasks(ctx)
		require.NoError(t, err)
	})

	t.Run("cancel tasks with non-existent IDs", func(t *testing.T) {
		err := repo.CancelTasks(ctx, "non_existent_1", "non_existent_2")
		require.NoError(t, err)
	})

	t.Run("cancel already cancelled task", func(t *testing.T) {
		cancelledTask := testDelegateTask
		cancelledTask.ID = "already_cancelled_task"
		cancelledTask.Status = domain.DelegateTaskStatusPending
		err := repo.Add(ctx, cancelledTask)
		require.NoError(t, err)

		err = repo.CancelTasks(ctx, cancelledTask.ID)
		require.NoError(t, err)

		err = repo.CancelTasks(ctx, cancelledTask.ID)
		require.NoError(t, err)

		task, err := repo.GetByID(ctx, cancelledTask.ID)
		require.NoError(t, err)
		require.Equal(t, domain.DelegateTaskStatusCancelled, task.Status)
	})

	t.Run("cancel task that is not pending", func(t *testing.T) {
		doneTask := testDelegateTask
		doneTask.ID = "done_task_to_cancel"
		doneTask.Status = domain.DelegateTaskStatusPending
		err := repo.Add(ctx, doneTask)
		require.NoError(t, err)
		err = repo.CompleteTasks(ctx, "commitment_txid_1", doneTask.ID)
		require.NoError(t, err)

		err = repo.CancelTasks(ctx, doneTask.ID)
		require.NoError(t, err)

		task, err := repo.GetByID(ctx, doneTask.ID)
		require.NoError(t, err)
		require.Equal(t, domain.DelegateTaskStatusCompleted, task.Status)
		require.Equal(t, "commitment_txid_1", task.CommitmentTxid)
	})
}

func testSuccessTasks(t *testing.T, repo domain.DelegateRepository) {
	t.Run("success tasks", func(t *testing.T) {
		successTask1 := testDelegateTask
		successTask1.ID = "success_task_1"
		successTask1.Status = domain.DelegateTaskStatusPending
		err := repo.Add(ctx, successTask1)
		require.NoError(t, err)

		successTask2 := testDelegateTask
		successTask2.ID = "success_task_2"
		successTask2.Status = domain.DelegateTaskStatusPending
		err = repo.Add(ctx, successTask2)
		require.NoError(t, err)

		task1, err := repo.GetByID(ctx, successTask1.ID)
		require.NoError(t, err)
		require.Equal(t, domain.DelegateTaskStatusPending, task1.Status)

		task2, err := repo.GetByID(ctx, successTask2.ID)
		require.NoError(t, err)
		require.Equal(t, domain.DelegateTaskStatusPending, task2.Status)

		err = repo.CompleteTasks(ctx, "commitment_txid_2", successTask1.ID, successTask2.ID)
		require.NoError(t, err)

		task1, err = repo.GetByID(ctx, successTask1.ID)
		require.NoError(t, err)
		require.Equal(t, domain.DelegateTaskStatusCompleted, task1.Status)
		require.Equal(t, "commitment_txid_2", task1.CommitmentTxid)

		task2, err = repo.GetByID(ctx, successTask2.ID)
		require.NoError(t, err)
		require.Equal(t, domain.DelegateTaskStatusCompleted, task2.Status)
		require.Equal(t, "commitment_txid_2", task2.CommitmentTxid)

		pendingTasks, err := repo.GetAllPending(ctx)
		require.NoError(t, err)
		for _, pendingTask := range pendingTasks {
			require.NotEqual(t, successTask1.ID, pendingTask.ID)
			require.NotEqual(t, successTask2.ID, pendingTask.ID)
		}
	})

	t.Run("success tasks with empty list", func(t *testing.T) {
		err := repo.CompleteTasks(ctx, "commitment_txid_3")
		require.NoError(t, err)
	})

	t.Run("success tasks with non-existent IDs", func(t *testing.T) {
		err := repo.CompleteTasks(ctx, "commitment_txid_4", "non_existent_1", "non_existent_2")
		require.NoError(t, err)
	})

	t.Run("success already done task", func(t *testing.T) {
		doneTask := testDelegateTask
		doneTask.ID = "already_done_task"
		doneTask.Status = domain.DelegateTaskStatusPending
		err := repo.Add(ctx, doneTask)
		require.NoError(t, err)

		err = repo.CompleteTasks(ctx, "commitment_txid_5", doneTask.ID)
		require.NoError(t, err)

		err = repo.CompleteTasks(ctx, "commitment_txid_6", doneTask.ID)
		require.NoError(t, err)

		task, err := repo.GetByID(ctx, doneTask.ID)
		require.NoError(t, err)
		require.Equal(t, domain.DelegateTaskStatusCompleted, task.Status)
		require.Equal(t, "commitment_txid_5", task.CommitmentTxid)
	})

	t.Run("success task that is not pending", func(t *testing.T) {
		cancelledTask := testDelegateTask
		cancelledTask.ID = "cancelled_task_to_success"
		cancelledTask.Status = domain.DelegateTaskStatusPending
		err := repo.Add(ctx, cancelledTask)
		require.NoError(t, err)
		err = repo.CancelTasks(ctx, cancelledTask.ID)
		require.NoError(t, err)

		err = repo.CompleteTasks(ctx, "commitment_txid_7", cancelledTask.ID)
		require.NoError(t, err)

		task, err := repo.GetByID(ctx, cancelledTask.ID)
		require.NoError(t, err)
		require.Equal(t, domain.DelegateTaskStatusCancelled, task.Status)
	})
}

func testFailTasks(t *testing.T, repo domain.DelegateRepository) {
	t.Run("fail tasks", func(t *testing.T) {
		failTask1 := testDelegateTask
		failTask1.ID = "fail_task_1"
		failTask1.Status = domain.DelegateTaskStatusPending
		err := repo.Add(ctx, failTask1)
		require.NoError(t, err)

		failTask2 := testDelegateTask
		failTask2.ID = "fail_task_2"
		failTask2.Status = domain.DelegateTaskStatusPending
		err = repo.Add(ctx, failTask2)
		require.NoError(t, err)

		task1, err := repo.GetByID(ctx, failTask1.ID)
		require.NoError(t, err)
		require.Equal(t, domain.DelegateTaskStatusPending, task1.Status)
		require.Empty(t, task1.FailReason)

		task2, err := repo.GetByID(ctx, failTask2.ID)
		require.NoError(t, err)
		require.Equal(t, domain.DelegateTaskStatusPending, task2.Status)
		require.Empty(t, task2.FailReason)

		failReason := "Transaction failed: insufficient funds"
		err = repo.FailTasks(ctx, failReason, failTask1.ID, failTask2.ID)
		require.NoError(t, err)

		task1, err = repo.GetByID(ctx, failTask1.ID)
		require.NoError(t, err)
		require.Equal(t, domain.DelegateTaskStatusFailed, task1.Status)
		require.Equal(t, failReason, task1.FailReason)

		task2, err = repo.GetByID(ctx, failTask2.ID)
		require.NoError(t, err)
		require.Equal(t, domain.DelegateTaskStatusFailed, task2.Status)
		require.Equal(t, failReason, task2.FailReason)

		pendingTasks, err := repo.GetAllPending(ctx)
		require.NoError(t, err)
		for _, pendingTask := range pendingTasks {
			require.NotEqual(t, failTask1.ID, pendingTask.ID)
			require.NotEqual(t, failTask2.ID, pendingTask.ID)
		}
	})

	t.Run("fail tasks with empty list", func(t *testing.T) {
		err := repo.FailTasks(ctx, "some reason")
		require.NoError(t, err)
	})

	t.Run("fail tasks with empty reason", func(t *testing.T) {
		failTask := testDelegateTask
		failTask.ID = "fail_task_empty_reason"
		failTask.Status = domain.DelegateTaskStatusPending
		err := repo.Add(ctx, failTask)
		require.NoError(t, err)

		err = repo.FailTasks(ctx, "", failTask.ID)
		require.NoError(t, err)

		task, err := repo.GetByID(ctx, failTask.ID)
		require.NoError(t, err)
		require.Equal(t, domain.DelegateTaskStatusFailed, task.Status)
		require.Empty(t, task.FailReason)
	})

	t.Run("fail tasks with non-existent IDs", func(t *testing.T) {
		err := repo.FailTasks(ctx, "some reason", "non_existent_1", "non_existent_2")
		require.NoError(t, err)
	})

	t.Run("fail task that is not pending", func(t *testing.T) {
		doneTask := testDelegateTask
		doneTask.ID = "done_task_to_fail"
		doneTask.Status = domain.DelegateTaskStatusPending
		err := repo.Add(ctx, doneTask)
		require.NoError(t, err)
		err = repo.CompleteTasks(ctx, "commitment_txid_1", doneTask.ID)
		require.NoError(t, err)

		err = repo.FailTasks(ctx, "some reason", doneTask.ID)
		require.NoError(t, err)

		task, err := repo.GetByID(ctx, doneTask.ID)
		require.NoError(t, err)
		require.Equal(t, domain.DelegateTaskStatusCompleted, task.Status)
		require.Equal(t, "commitment_txid_1", task.CommitmentTxid)
	})

	t.Run("fail task with long reason", func(t *testing.T) {
		failTask := testDelegateTask
		failTask.ID = "fail_task_long_reason"
		failTask.Status = domain.DelegateTaskStatusPending
		err := repo.Add(ctx, failTask)
		require.NoError(t, err)

		longReason := "This is a very long failure reason that might contain detailed error information, stack traces, or other diagnostic data that helps understand why the task failed. It could be multiple lines or contain special characters."
		err = repo.FailTasks(ctx, longReason, failTask.ID)
		require.NoError(t, err)

		task, err := repo.GetByID(ctx, failTask.ID)
		require.NoError(t, err)
		require.Equal(t, domain.DelegateTaskStatusFailed, task.Status)
		require.Equal(t, longReason, task.FailReason)
	})
}

func testAddSwap(t *testing.T, repo domain.SwapRepository) {
	t.Run("add swap", func(t *testing.T) {
		swap, err := repo.Get(ctx, testSwap.Id)
		require.Error(t, err)
		require.Nil(t, swap)

		count, err := repo.Add(ctx, []domain.Swap{testSwap})
		require.NoError(t, err)
		require.Equal(t, 1, count)

		count, err = repo.Add(ctx, []domain.Swap{testSwap})
		require.NoError(t, err)
		require.LessOrEqual(t, 0, count)

		swap, err = repo.Get(ctx, testSwap.Id)
		require.NoError(t, err)
		require.NotNil(t, swap)
		require.Equal(t, *swap, testSwap)
	})
}

func testGetAllSwap(t *testing.T, repo domain.SwapRepository) {
	t.Run("get all swaps", func(t *testing.T) {
		swaps, err := repo.GetAll(ctx)
		require.NoError(t, err)
		require.Len(t, swaps, 1)

		count, err := repo.Add(ctx, []domain.Swap{testSwap, secondSwap})
		require.NoError(t, err)
		require.Equal(t, 1, count)

		count, err = repo.Add(ctx, []domain.Swap{testSwap, secondSwap})
		require.NoError(t, err)
		require.LessOrEqual(t, 0, count)

		swaps, err = repo.GetAll(ctx)
		require.NoError(t, err)
		require.Len(t, swaps, 2)
		require.Subset(t, []domain.Swap{testSwap, secondSwap}, swaps)
	})
}

func testUpdateSwap(t *testing.T, repo domain.SwapRepository) {
	t.Run("update swap", func(t *testing.T) {
		modifiedTestSwap := testSwap
		modifiedTestSwap.Status = domain.SwapSuccess
		modifiedTestSwap.RedeemTxId = "redeemed_tx_id"

		err := repo.Update(ctx, modifiedTestSwap)
		require.NoError(t, err)

		updatedSwap, err := repo.Get(ctx, testSwap.Id)
		require.NoError(t, err)
		require.NotNil(t, updatedSwap)
		require.Equal(t, domain.SwapSuccess, updatedSwap.Status)
		require.Equal(t, "redeemed_tx_id", updatedSwap.RedeemTxId)
	})
}

func testAddSubscribedScripts(t *testing.T, repo domain.SubscribedScriptRepository) {
	t.Run("add subscribed scripts", func(t *testing.T) {
		scripts, err := repo.Get(ctx)
		require.NoError(t, err)
		require.Empty(t, scripts)

		count, err := repo.Add(ctx, testSubscribedScripts)
		require.NoError(t, err)
		require.Equal(t, len(testSubscribedScripts), count)

		scripts, err = repo.Get(ctx)
		require.NoError(t, err)
		require.ElementsMatch(t, testSubscribedScripts, scripts)

		count, err = repo.Add(ctx, testSubscribedScripts)
		require.NoError(t, err)
		require.Equal(t, 0, count)

	})
}

func testDeleteSubscribedScripts(t *testing.T, repo domain.SubscribedScriptRepository) {
	t.Run("delete subscribed scripts", func(t *testing.T) {
		scripts, err := repo.Get(ctx)
		require.NoError(t, err)
		require.ElementsMatch(t, testSubscribedScripts, scripts)

		test2SubscribedScripts := []string{
			"script4",
			"script5",
			"script6",
		}
		count, err := repo.Add(ctx, test2SubscribedScripts)
		require.NoError(t, err)
		require.Equal(t, len(test2SubscribedScripts), count)

		scripts, err = repo.Get(ctx)
		require.NoError(t, err)

		require.ElementsMatch(t, append(testSubscribedScripts, test2SubscribedScripts...), scripts)

		count, err = repo.Delete(ctx, test2SubscribedScripts)
		require.NoError(t, err)
		require.Equal(t, len(test2SubscribedScripts), count)

		scripts, err = repo.Get(ctx)
		require.NoError(t, err)
		require.ElementsMatch(t, testSubscribedScripts, scripts)

		count, err = repo.Delete(ctx, test2SubscribedScripts)
		require.NoError(t, err)
		require.Equal(t, 0, count)
	})

}

func makeVHTLC() domain.Vhtlc {
	randBytes := make([]byte, 20)
	_, _ = rand.Read(randBytes)

	serverKey, _ := btcec.NewPrivateKey()
	senderKey, _ := btcec.NewPrivateKey()
	receiverKey, _ := btcec.NewPrivateKey()

	opts := vhtlc.Opts{
		PreimageHash:   randBytes,
		Sender:         senderKey.PubKey(),
		Receiver:       receiverKey.PubKey(),
		Server:         serverKey.PubKey(),
		RefundLocktime: arklib.AbsoluteLocktime(100 * 600),
		UnilateralClaimDelay: arklib.RelativeLocktime{
			Type:  arklib.LocktimeTypeBlock,
			Value: 300,
		},
		UnilateralRefundDelay: arklib.RelativeLocktime{
			Type:  arklib.LocktimeTypeBlock,
			Value: 400,
		},
		UnilateralRefundWithoutReceiverDelay: arklib.RelativeLocktime{
			Type:  arklib.LocktimeTypeBlock,
			Value: 500,
		},
	}

	return domain.NewVhtlc(opts)
}

func makeSwap() domain.Swap {
	return domain.Swap{
		Id:          uuid.New().String(),
		Amount:      1000,
		Timestamp:   time.Now().Unix(),
		To:          "test_to",
		From:        "test_from",
		Status:      domain.SwapSuccess,
		Type:        domain.SwapPayment,
		Invoice:     "test_invoice",
		Vhtlc:       makeVHTLC(),
		FundingTxId: "funding_tx_id",
		RedeemTxId:  "redeem_tx_id",
	}
}
