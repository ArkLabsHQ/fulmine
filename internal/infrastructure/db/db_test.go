package db_test

import (
	"context"
	"crypto/rand"
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
		return domain.DelegateTask{
			ID:                "test_task_id",
			Intent:            domain.Intent{Message: "test_message", Proof: "test_proof"},
			ForfeitTx:         "forfeit_tx_hex",
			Input:             wire.OutPoint{Hash: *hash1, Index: 0},
			Fee:               1000,
			DelegatorPublicKey: "delegator_pubkey",
			ScheduledAt:       time.Now(),
			Status:            domain.DelegateTaskStatusPending,
			FailReason:        "",
		}
	}()
	secondDelegateTask = func() domain.DelegateTask {
		hash2, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000002")
		return domain.DelegateTask{
			ID:                "second_task_id",
			Intent:            domain.Intent{Message: "second_message", Proof: "second_proof"},
			ForfeitTx:         "second_forfeit_tx_hex",
			Input:             wire.OutPoint{Hash: *hash2, Index: 1},
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
		testAddOrUpdateDelegateTask(t, svc.Delegate())
		testGetDelegateTaskByID(t, svc.Delegate())
		testGetAllPendingDelegateTasks(t, svc.Delegate())
		testGetPendingTaskByInput(t, svc.Delegate())
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

func testAddOrUpdateDelegateTask(t *testing.T, repo domain.DelegatorRepository) {
	t.Run("add or update delegate task", func(t *testing.T) {
		// Task should not exist initially
		task, err := repo.GetByID(ctx, testDelegateTask.ID)
		require.Error(t, err)
		require.Nil(t, task)

		// Add new task
		err = repo.AddOrUpdate(ctx, testDelegateTask)
		require.NoError(t, err)

		// Verify the task was added correctly
		task, err = repo.GetByID(ctx, testDelegateTask.ID)
		require.NoError(t, err)
		require.NotNil(t, task)
		require.Equal(t, testDelegateTask.ID, task.ID)
		require.Equal(t, testDelegateTask.Intent, task.Intent)
		require.Equal(t, testDelegateTask.Status, task.Status)

		// Update the task
		updatedTask := testDelegateTask
		updatedTask.Status = domain.DelegateTaskStatusDone
		updatedTask.FailReason = "completed"
		err = repo.AddOrUpdate(ctx, updatedTask)
		require.NoError(t, err)

		// Verify the task was updated
		task, err = repo.GetByID(ctx, testDelegateTask.ID)
		require.NoError(t, err)
		require.NotNil(t, task)
		require.Equal(t, domain.DelegateTaskStatusDone, task.Status)
		require.Equal(t, "completed", task.FailReason)
	})
}

func testGetDelegateTaskByID(t *testing.T, repo domain.DelegatorRepository) {
	t.Run("get delegate task by id", func(t *testing.T) {
		// Reset task to pending for this test
		testTask := testDelegateTask
		testTask.Status = domain.DelegateTaskStatusPending
		err := repo.AddOrUpdate(ctx, testTask)
		require.NoError(t, err)

		// Get task by ID
		task, err := repo.GetByID(ctx, testTask.ID)
		require.NoError(t, err)
		require.NotNil(t, task)
		require.Equal(t, testTask.ID, task.ID)
		require.Equal(t, testTask.Intent, task.Intent)

		// Try to get non-existent task
		_, err = repo.GetByID(ctx, "non_existent_id")
		require.Error(t, err)
	})
}

func testGetAllPendingDelegateTasks(t *testing.T, repo domain.DelegatorRepository) {
	t.Run("get all pending delegate tasks", func(t *testing.T) {
		// Add a pending task
		pendingTask := testDelegateTask
		pendingTask.ID = "pending_task_1"
		pendingTask.Status = domain.DelegateTaskStatusPending
		err := repo.AddOrUpdate(ctx, pendingTask)
		require.NoError(t, err)

		// Add a done task (should not appear in GetAllPending)
		doneTask := secondDelegateTask
		doneTask.ID = "done_task_1"
		doneTask.Status = domain.DelegateTaskStatusDone
		err = repo.AddOrUpdate(ctx, doneTask)
		require.NoError(t, err)

		// Add another pending task
		anotherPendingTask := secondDelegateTask
		anotherPendingTask.ID = "pending_task_2"
		anotherPendingTask.Status = domain.DelegateTaskStatusPending
		err = repo.AddOrUpdate(ctx, anotherPendingTask)
		require.NoError(t, err)

		// Get all pending tasks
		pendingTasks, err := repo.GetAllPending(ctx)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(pendingTasks), 2)

		// Verify specific pending tasks are included
		taskIDs := make(map[string]bool)
		for _, pendingTask := range pendingTasks {
			taskIDs[pendingTask.ID] = true
			// Verify that we can fetch the full task and it's pending
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

func testGetPendingTaskByInput(t *testing.T, repo domain.DelegatorRepository) {
	t.Run("get pending task by input", func(t *testing.T) {
		// Create a unique input for this test
		hash1, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000003")
		testInput := wire.OutPoint{Hash: *hash1, Index: 0}

		// Add a pending task with the test input
		pendingTask1 := testDelegateTask
		pendingTask1.ID = "pending_by_input_1"
		pendingTask1.Input = testInput
		pendingTask1.Status = domain.DelegateTaskStatusPending
		err := repo.AddOrUpdate(ctx, pendingTask1)
		require.NoError(t, err)

		// Add another pending task with the same input
		pendingTask2 := testDelegateTask
		pendingTask2.ID = "pending_by_input_2"
		pendingTask2.Input = testInput
		pendingTask2.Status = domain.DelegateTaskStatusPending
		err = repo.AddOrUpdate(ctx, pendingTask2)
		require.NoError(t, err)

		// Add a pending task with a different input (should not be returned)
		hash2, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000004")
		differentInput := wire.OutPoint{Hash: *hash2, Index: 0}
		pendingTask3 := testDelegateTask
		pendingTask3.ID = "pending_by_input_3"
		pendingTask3.Input = differentInput
		pendingTask3.Status = domain.DelegateTaskStatusPending
		err = repo.AddOrUpdate(ctx, pendingTask3)
		require.NoError(t, err)

		// Add a done task with the same input (should not be returned)
		doneTask := testDelegateTask
		doneTask.ID = "done_by_input_1"
		doneTask.Input = testInput
		doneTask.Status = domain.DelegateTaskStatusDone
		err = repo.AddOrUpdate(ctx, doneTask)
		require.NoError(t, err)

		// Get pending tasks by input
		tasks, err := repo.GetPendingTaskByInput(ctx, testInput)
		require.NoError(t, err)
		require.Len(t, tasks, 2)

		// Verify the returned tasks
		taskIDs := make(map[string]bool)
		for _, task := range tasks {
			taskIDs[task.ID] = true
			require.Equal(t, domain.DelegateTaskStatusPending, task.Status)
			require.Equal(t, testInput.Hash.String(), task.Input.Hash.String())
			require.Equal(t, testInput.Index, task.Input.Index)
		}
		require.True(t, taskIDs["pending_by_input_1"])
		require.True(t, taskIDs["pending_by_input_2"])
		require.False(t, taskIDs["pending_by_input_3"])
		require.False(t, taskIDs["done_by_input_1"])

		// Test with a non-existent input
		hash3, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000005")
		nonExistentInput := wire.OutPoint{Hash: *hash3, Index: 0}
		tasks, err = repo.GetPendingTaskByInput(ctx, nonExistentInput)
		require.NoError(t, err)
		require.Len(t, tasks, 0)
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

		// Add another swap

		count, err := repo.Add(ctx, []domain.Swap{testSwap, secondSwap})
		require.NoError(t, err)
		require.Equal(t, 1, count)

		count, err = repo.Add(ctx, []domain.Swap{testSwap, secondSwap})
		require.NoError(t, err)
		require.LessOrEqual(t, 0, count)

		// Get all swaps
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
