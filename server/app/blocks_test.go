package app

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	mmModel "github.com/mattermost/mattermost-server/v6/model"

	"github.com/mattermost/focalboard/server/model"
)

type blockError struct {
	msg string
}

func (be blockError) Error() string {
	return be.msg
}

func TestInsertBlock(t *testing.T) {
	th, tearDown := SetupTestHelper(t)
	defer tearDown()

	t.Run("success scenario", func(t *testing.T) {
		boardID := testBoardID
		block := &model.Block{BoardID: boardID}
		board := &model.Board{ID: boardID}
		th.Store.EXPECT().GetBoard(boardID).Return(board, nil)
		th.Store.EXPECT().InsertBlock(block, "user-id-1").Return(nil)
		th.Store.EXPECT().GetMembersForBoard(boardID).Return([]*model.BoardMember{}, nil)
		err := th.App.InsertBlock(block, "user-id-1")
		require.NoError(t, err)
	})

	t.Run("error scenario", func(t *testing.T) {
		boardID := testBoardID
		block := &model.Block{BoardID: boardID}
		board := &model.Board{ID: boardID}
		th.Store.EXPECT().GetBoard(boardID).Return(board, nil)
		th.Store.EXPECT().InsertBlock(block, "user-id-1").Return(blockError{"error"})
		err := th.App.InsertBlock(block, "user-id-1")
		require.Error(t, err, "error")
	})
}

func TestPatchBlocks(t *testing.T) {
	th, tearDown := SetupTestHelper(t)
	defer tearDown()

	t.Run("patchBlocks success scenario", func(t *testing.T) {
		blockPatches := model.BlockPatchBatch{
			BlockIDs: []string{"block1"},
			BlockPatches: []model.BlockPatch{
				{Title: mmModel.NewString("new title")},
			},
		}

		block1 := &model.Block{ID: "block1"}
		th.Store.EXPECT().GetBlocksByIDs([]string{"block1"}).Return([]*model.Block{block1}, nil)
		th.Store.EXPECT().PatchBlocks(gomock.Eq(&blockPatches), gomock.Eq("user-id-1")).Return(nil)
		th.Store.EXPECT().GetBlock("block1").Return(block1, nil)
		// this call comes from the WS server notification
		th.Store.EXPECT().GetMembersForBoard(gomock.Any()).Times(1)
		err := th.App.PatchBlocks("team-id", &blockPatches, "user-id-1")
		require.NoError(t, err)
	})

	t.Run("patchBlocks error scenario", func(t *testing.T) {
		blockPatches := model.BlockPatchBatch{BlockIDs: []string{}}
		th.Store.EXPECT().GetBlocksByIDs([]string{}).Return(nil, sql.ErrNoRows)
		err := th.App.PatchBlocks("team-id", &blockPatches, "user-id-1")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("cloud limit error scenario", func(t *testing.T) {
		t.Skipf("The Cloud Limits feature has been disabled")

		th.App.SetCardLimit(5)

		fakeLicense := &mmModel.License{
			Features: &mmModel.Features{Cloud: mmModel.NewBool(true)},
		}

		blockPatches := model.BlockPatchBatch{
			BlockIDs: []string{"block1"},
			BlockPatches: []model.BlockPatch{
				{Title: mmModel.NewString("new title")},
			},
		}

		block1 := &model.Block{
			ID:       "block1",
			Type:     model.TypeCard,
			ParentID: "board-id",
			BoardID:  "board-id",
			UpdateAt: 100,
		}

		board1 := &model.Board{
			ID:   "board-id",
			Type: model.BoardTypeOpen,
		}

		th.Store.EXPECT().GetBlocksByIDs([]string{"block1"}).Return([]*model.Block{block1}, nil)
		th.Store.EXPECT().GetBoard("board-id").Return(board1, nil)
		th.Store.EXPECT().GetLicense().Return(fakeLicense)
		th.Store.EXPECT().GetCardLimitTimestamp().Return(int64(150), nil)
		err := th.App.PatchBlocks("team-id", &blockPatches, "user-id-1")
		require.ErrorIs(t, err, model.ErrPatchUpdatesLimitedCards)
	})
}

func TestDeleteBlock(t *testing.T) {
	th, tearDown := SetupTestHelper(t)
	defer tearDown()

	t.Run("success scenario", func(t *testing.T) {
		boardID := testBoardID
		board := &model.Board{ID: boardID}
		block := &model.Block{
			ID:      "block-id",
			BoardID: board.ID,
		}
		th.Store.EXPECT().GetBlock(gomock.Eq("block-id")).Return(block, nil)
		th.Store.EXPECT().DeleteBlock(gomock.Eq("block-id"), gomock.Eq("user-id-1")).Return(nil)
		th.Store.EXPECT().GetBoard(gomock.Eq(testBoardID)).Return(board, nil)
		th.Store.EXPECT().GetMembersForBoard(boardID).Return([]*model.BoardMember{}, nil)
		err := th.App.DeleteBlock("block-id", "user-id-1")
		require.NoError(t, err)
	})

	t.Run("card soft delete keeps task ids reserved", func(t *testing.T) {
		boardID := testBoardID
		board := &model.Board{ID: boardID}
		block := &model.Block{
			ID:      "card-3090",
			BoardID: board.ID,
			Type:    model.TypeCard,
			Fields: map[string]interface{}{
				"taskId":       "#3090",
				"globalTaskId": "G-3090",
			},
		}
		th.App.cardTaskIDMaxByBoard[boardID] = 3090
		th.App.cardTaskIDBackfilledBoards[boardID] = true
		th.App.cardGlobalTaskIDMax = 3090
		th.App.cardGlobalTaskIDCounterLoaded = true

		th.Store.EXPECT().GetBlock(gomock.Eq("card-3090")).Return(block, nil)
		th.Store.EXPECT().DeleteBlock(gomock.Eq("card-3090"), gomock.Eq("user-id-1")).Return(nil)
		th.Store.EXPECT().GetBoard(gomock.Eq(testBoardID)).Return(board, nil)
		th.Store.EXPECT().GetMembersForBoard(boardID).Return([]*model.BoardMember{}, nil)

		err := th.App.DeleteBlock("card-3090", "user-id-1")

		require.NoError(t, err)
		require.Equal(t, 3090, th.App.cardTaskIDMaxByBoard[boardID])
		require.Equal(t, 3090, th.App.cardGlobalTaskIDMax)
	})

	t.Run("error scenario", func(t *testing.T) {
		boardID := testBoardID
		board := &model.Board{ID: boardID}
		block := &model.Block{
			ID:      "block-id",
			BoardID: board.ID,
		}
		th.Store.EXPECT().GetBlock(gomock.Eq("block-id")).Return(block, nil)
		th.Store.EXPECT().DeleteBlock(gomock.Eq("block-id"), gomock.Eq("user-id-1")).Return(blockError{"error"})
		th.Store.EXPECT().GetBoard(gomock.Eq(testBoardID)).Return(board, nil)
		err := th.App.DeleteBlock("block-id", "user-id-1")
		require.Error(t, err, "error")
	})
}

func TestUndeleteBlock(t *testing.T) {
	th, tearDown := SetupTestHelper(t)
	defer tearDown()

	t.Run("success scenario", func(t *testing.T) {
		boardID := testBoardID
		board := &model.Board{ID: boardID}
		block := &model.Block{
			ID:      "block-id",
			BoardID: board.ID,
		}
		th.Store.EXPECT().GetBlockHistory(
			gomock.Eq("block-id"),
			gomock.Eq(model.QueryBlockHistoryOptions{Limit: 1, Descending: true}),
		).Return([]*model.Block{block}, nil)
		th.Store.EXPECT().UndeleteBlock(gomock.Eq("block-id"), gomock.Eq("user-id-1")).Return(nil)
		th.Store.EXPECT().GetBlock(gomock.Eq("block-id")).Return(block, nil)
		th.Store.EXPECT().GetBoard(boardID).Return(board, nil)
		th.Store.EXPECT().GetMembersForBoard(boardID).Return([]*model.BoardMember{}, nil)
		_, err := th.App.UndeleteBlock("block-id", "user-id-1")
		require.NoError(t, err)
	})

	t.Run("card undelete gets new ids when released ids were reused", func(t *testing.T) {
		boardID := testBoardID
		board := &model.Board{ID: boardID}
		block := &model.Block{
			ID:      "old-card-3090",
			BoardID: board.ID,
			Type:    model.TypeCard,
			Fields: map[string]interface{}{
				"taskId":       "#3090",
				"globalTaskId": "G-3090",
			},
		}
		activeBlock := &model.Block{
			ID:      "new-card-3090",
			BoardID: board.ID,
			Type:    model.TypeCard,
			Fields: map[string]interface{}{
				"taskId":       "#3090",
				"globalTaskId": "G-3090",
			},
		}
		expectedProperties := map[string]interface{}{cardGlobalTaskIDProperty: "G-3091"}

		th.Store.EXPECT().GetBlockHistory(
			gomock.Eq("old-card-3090"),
			gomock.Eq(model.QueryBlockHistoryOptions{Limit: 1, Descending: true}),
		).Return([]*model.Block{block}, nil)
		th.Store.EXPECT().UndeleteBlock(gomock.Eq("old-card-3090"), gomock.Eq("user-id-1")).Return(nil)
		th.Store.EXPECT().GetBlock(gomock.Eq("old-card-3090")).Return(block, nil)
		th.Store.EXPECT().GetBlocks(model.QueryBlocksOptions{
			BoardID:   boardID,
			BlockType: model.TypeCard,
		}).Return([]*model.Block{activeBlock, block}, nil)
		th.Store.EXPECT().GetBlocks(model.QueryBlocksOptions{
			BoardID:   boardID,
			BlockType: model.TypeCard,
		}).Return([]*model.Block{activeBlock, block}, nil)
		th.Store.EXPECT().GetDeletedBlocksForBoard(boardID).Return([]*model.Block{}, nil)
		th.Store.EXPECT().GetSystemSetting(cardTaskIDCounterKey(boardID)).Return("", nil)
		th.Store.EXPECT().GetBlocks(model.QueryBlocksOptions{
			BlockType: model.TypeCard,
		}).Return([]*model.Block{activeBlock, block}, nil)
		th.Store.EXPECT().GetBlocks(model.QueryBlocksOptions{
			BlockType: model.TypeCard,
		}).Return([]*model.Block{activeBlock, block}, nil)
		th.Store.EXPECT().GetDeletedBlocksWithType(string(model.TypeCard)).Return([]*model.Block{}, nil)
		th.Store.EXPECT().SetSystemSetting(cardGlobalTaskIDCounterKey, "3090").Return(nil)
		th.Store.EXPECT().GetSystemSetting(cardGlobalTaskIDCounterKey).Return("", nil)
		th.Store.EXPECT().SetSystemSetting(cardTaskIDCounterKey(boardID), "3091").Return(nil)
		th.Store.EXPECT().SetSystemSetting(cardGlobalTaskIDCounterKey, "3091").Return(nil)
		th.Store.EXPECT().PatchBlock("old-card-3090", &model.BlockPatch{
			UpdatedFields: map[string]interface{}{
				"taskId":       "#3091",
				"globalTaskId": "G-3091",
				"properties":   expectedProperties,
			},
		}, "user-id-1").Return(nil)
		th.Store.EXPECT().GetBoard(boardID).Return(board, nil)
		th.Store.EXPECT().GetMembersForBoard(boardID).Return([]*model.BoardMember{}, nil)

		undeletedBlock, err := th.App.UndeleteBlock("old-card-3090", "user-id-1")

		require.NoError(t, err)
		require.Equal(t, "#3091", undeletedBlock.Fields["taskId"])
		require.Equal(t, "G-3091", undeletedBlock.Fields["globalTaskId"])
		require.Equal(t, expectedProperties, undeletedBlock.Fields["properties"])
	})

	t.Run("card undelete does not lower counters reserved by deleted history", func(t *testing.T) {
		boardID := testBoardID
		board := &model.Board{ID: boardID}
		block := &model.Block{
			ID:      "old-card-100",
			BoardID: board.ID,
			Type:    model.TypeCard,
			Fields: map[string]interface{}{
				"taskId":       "#100",
				"globalTaskId": "G-100",
			},
		}
		activeBlock := &model.Block{
			ID:      "active-card-200",
			BoardID: board.ID,
			Type:    model.TypeCard,
			Fields: map[string]interface{}{
				"taskId":       "#200",
				"globalTaskId": "G-200",
			},
		}
		deletedHighestBlock := &model.Block{
			ID:       "deleted-card-36666",
			BoardID:  board.ID,
			Type:     model.TypeCard,
			DeleteAt: 1680000000000,
			Fields: map[string]interface{}{
				"taskId":       "#36666",
				"globalTaskId": "G-36666",
			},
		}
		th.App.cardTaskIDMaxByBoard[boardID] = 36666
		th.App.cardGlobalTaskIDMax = 36666
		th.App.cardGlobalTaskIDCounterLoaded = true

		th.Store.EXPECT().GetBlockHistory(
			gomock.Eq("old-card-100"),
			gomock.Eq(model.QueryBlockHistoryOptions{Limit: 1, Descending: true}),
		).Return([]*model.Block{block}, nil)
		th.Store.EXPECT().UndeleteBlock(gomock.Eq("old-card-100"), gomock.Eq("user-id-1")).Return(nil)
		th.Store.EXPECT().GetBlock(gomock.Eq("old-card-100")).Return(block, nil)
		th.Store.EXPECT().GetBlocks(model.QueryBlocksOptions{
			BoardID:   boardID,
			BlockType: model.TypeCard,
		}).Return([]*model.Block{activeBlock, block}, nil)
		th.Store.EXPECT().GetBlocks(model.QueryBlocksOptions{
			BoardID:   boardID,
			BlockType: model.TypeCard,
		}).Return([]*model.Block{activeBlock, block}, nil)
		th.Store.EXPECT().GetDeletedBlocksForBoard(boardID).Return([]*model.Block{deletedHighestBlock}, nil)
		th.Store.EXPECT().GetSystemSetting(cardTaskIDCounterKey(boardID)).Return("36666", nil)
		th.Store.EXPECT().GetBlocks(model.QueryBlocksOptions{
			BlockType: model.TypeCard,
		}).Return([]*model.Block{activeBlock, block}, nil)
		th.Store.EXPECT().GetBlocks(model.QueryBlocksOptions{
			BlockType: model.TypeCard,
		}).Return([]*model.Block{activeBlock, block}, nil)
		th.Store.EXPECT().GetDeletedBlocksWithType(string(model.TypeCard)).Return([]*model.Block{deletedHighestBlock}, nil)
		th.Store.EXPECT().SetSystemSetting(cardGlobalTaskIDCounterKey, "36666").Return(nil)
		th.Store.EXPECT().GetSystemSetting(cardGlobalTaskIDCounterKey).Return("36666", nil)
		th.Store.EXPECT().SetSystemSetting(cardTaskIDCounterKey(boardID), "36666").Return(nil)
		th.Store.EXPECT().SetSystemSetting(cardGlobalTaskIDCounterKey, "36666").Return(nil)
		th.Store.EXPECT().PatchBlock("old-card-100", &model.BlockPatch{
			UpdatedFields: map[string]interface{}{
				"properties": map[string]interface{}{cardGlobalTaskIDProperty: "G-100"},
			},
		}, "user-id-1").Return(nil)
		th.Store.EXPECT().GetBoard(boardID).Return(board, nil)
		th.Store.EXPECT().GetMembersForBoard(boardID).Return([]*model.BoardMember{}, nil)

		undeletedBlock, err := th.App.UndeleteBlock("old-card-100", "user-id-1")

		require.NoError(t, err)
		require.Equal(t, "#100", undeletedBlock.Fields["taskId"])
		require.Equal(t, "G-100", undeletedBlock.Fields["globalTaskId"])
		require.Equal(t, 36666, th.App.cardTaskIDMaxByBoard[boardID])
		require.Equal(t, 36666, th.App.cardGlobalTaskIDMax)
	})

	t.Run("error scenario", func(t *testing.T) {
		block := &model.Block{
			ID: "block-id",
		}
		th.Store.EXPECT().GetBlockHistory(
			gomock.Eq("block-id"),
			gomock.Eq(model.QueryBlockHistoryOptions{Limit: 1, Descending: true}),
		).Return([]*model.Block{block}, nil)
		th.Store.EXPECT().UndeleteBlock(gomock.Eq("block-id"), gomock.Eq("user-id-1")).Return(blockError{"error"})
		_, err := th.App.UndeleteBlock("block-id", "user-id-1")
		require.Error(t, err, "error")
	})
}

func TestPermanentlyDeleteBlock(t *testing.T) {
	th, tearDown := SetupTestHelper(t)
	defer tearDown()

	t.Run("success scenario", func(t *testing.T) {
		boardID := testBoardID
		board := &model.Board{ID: boardID}
		block := &model.Block{
			ID:       "deleted-card-id",
			BoardID:  board.ID,
			Type:     model.TypeCard,
			DeleteAt: 1680000000000,
		}

		th.Store.EXPECT().GetBlockHistory(
			gomock.Eq("deleted-card-id"),
			gomock.Eq(model.QueryBlockHistoryOptions{Limit: 1, Descending: true}),
		).Return([]*model.Block{block}, nil)
		th.Store.EXPECT().GetBoard(boardID).Return(board, nil)
		th.Store.EXPECT().PermanentlyDeleteBlock(gomock.Eq("deleted-card-id"), gomock.Eq("user-id-1")).Return(nil)
		th.Store.EXPECT().GetMembersForBoard(boardID).Return([]*model.BoardMember{}, nil)

		deletedBlock, err := th.App.PermanentlyDeleteBlock("deleted-card-id", "user-id-1")

		require.NoError(t, err)
		require.Equal(t, block, deletedBlock)
	})

	t.Run("rejects active block", func(t *testing.T) {
		block := &model.Block{
			ID:      "active-card-id",
			BoardID: testBoardID,
			Type:    model.TypeCard,
		}

		th.Store.EXPECT().GetBlockHistory(
			gomock.Eq("active-card-id"),
			gomock.Eq(model.QueryBlockHistoryOptions{Limit: 1, Descending: true}),
		).Return([]*model.Block{block}, nil)

		_, err := th.App.PermanentlyDeleteBlock("active-card-id", "user-id-1")

		require.Error(t, err)
		require.True(t, model.IsErrBadRequest(err))
	})

	t.Run("card permanent delete does not lower persisted counters", func(t *testing.T) {
		boardID := testBoardID
		board := &model.Board{ID: boardID}
		block := &model.Block{
			ID:       "deleted-card-100",
			BoardID:  board.ID,
			Type:     model.TypeCard,
			DeleteAt: 1680000000000,
			Fields: map[string]interface{}{
				"taskId":       "#100",
				"globalTaskId": "G-100",
			},
		}
		activeBlock := &model.Block{
			ID:      "active-card-200",
			BoardID: board.ID,
			Type:    model.TypeCard,
			Fields: map[string]interface{}{
				"taskId":       "#200",
				"globalTaskId": "G-200",
			},
		}
		deletedHighestBlock := &model.Block{
			ID:       "deleted-card-36666",
			BoardID:  board.ID,
			Type:     model.TypeCard,
			DeleteAt: 1680000000000,
			Fields: map[string]interface{}{
				"taskId":       "#36666",
				"globalTaskId": "G-36666",
			},
		}
		th.App.cardTaskIDMaxByBoard[boardID] = 36666
		th.App.cardGlobalTaskIDMax = 36666
		th.App.cardGlobalTaskIDCounterLoaded = true

		th.Store.EXPECT().GetBlockHistory(
			gomock.Eq("deleted-card-100"),
			gomock.Eq(model.QueryBlockHistoryOptions{Limit: 1, Descending: true}),
		).Return([]*model.Block{block}, nil)
		th.Store.EXPECT().GetBoard(boardID).Return(board, nil)
		th.Store.EXPECT().GetBlocks(model.QueryBlocksOptions{
			BoardID:   boardID,
			BlockType: model.TypeCard,
		}).Return([]*model.Block{activeBlock}, nil)
		th.Store.EXPECT().GetDeletedBlocksForBoard(boardID).Return([]*model.Block{block, deletedHighestBlock}, nil)
		th.Store.EXPECT().GetSystemSetting(cardTaskIDCounterKey(boardID)).Return("36666", nil)
		th.Store.EXPECT().SetSystemSetting(cardTaskIDCounterKey(boardID), "36666").Return(nil)
		th.Store.EXPECT().GetBlocks(model.QueryBlocksOptions{
			BlockType: model.TypeCard,
		}).Return([]*model.Block{activeBlock}, nil)
		th.Store.EXPECT().GetDeletedBlocksWithType(string(model.TypeCard)).Return([]*model.Block{block, deletedHighestBlock}, nil)
		th.Store.EXPECT().SetSystemSetting(cardGlobalTaskIDCounterKey, "36666").Return(nil)
		th.Store.EXPECT().GetSystemSetting(cardGlobalTaskIDCounterKey).Return("36666", nil)
		th.Store.EXPECT().SetSystemSetting(cardGlobalTaskIDCounterKey, "36666").Return(nil)
		th.Store.EXPECT().PermanentlyDeleteBlock(gomock.Eq("deleted-card-100"), gomock.Eq("user-id-1")).Return(nil)
		th.Store.EXPECT().GetMembersForBoard(boardID).Return([]*model.BoardMember{}, nil)

		deletedBlock, err := th.App.PermanentlyDeleteBlock("deleted-card-100", "user-id-1")

		require.NoError(t, err)
		require.Equal(t, block, deletedBlock)
		require.Equal(t, 36666, th.App.cardTaskIDMaxByBoard[boardID])
		require.Equal(t, 36666, th.App.cardGlobalTaskIDMax)
	})
}

func TestIsWithinViewsLimit(t *testing.T) {
	t.Skipf("The Cloud Limits feature has been disabled")

	th, tearDown := SetupTestHelper(t)
	defer tearDown()

	fakeLicense := &mmModel.License{
		Features: &mmModel.Features{Cloud: mmModel.NewBool(true)},
	}

	t.Run("within views limit", func(t *testing.T) {
		th.Store.EXPECT().GetLicense().Return(fakeLicense)

		cloudLimit := &mmModel.ProductLimits{
			Boards: &mmModel.BoardsLimits{
				Views: mmModel.NewInt(2),
			},
		}
		th.Store.EXPECT().GetCloudLimits().Return(cloudLimit, nil)
		th.Store.EXPECT().GetUsedCardsCount().Return(1, nil)
		th.Store.EXPECT().GetCardLimitTimestamp().Return(int64(1), nil)
		th.Store.EXPECT().GetBlocksWithParentAndType("board_id", "parent_id", "view").Return([]*model.Block{{}}, nil)

		withinLimits, err := th.App.isWithinViewsLimit("board_id", &model.Block{ParentID: "parent_id"})
		assert.NoError(t, err)
		assert.True(t, withinLimits)
	})

	t.Run("view limit exactly reached", func(t *testing.T) {
		th.Store.EXPECT().GetLicense().Return(fakeLicense)

		cloudLimit := &mmModel.ProductLimits{
			Boards: &mmModel.BoardsLimits{
				Views: mmModel.NewInt(1),
			},
		}
		th.Store.EXPECT().GetCloudLimits().Return(cloudLimit, nil)
		th.Store.EXPECT().GetUsedCardsCount().Return(1, nil)
		th.Store.EXPECT().GetCardLimitTimestamp().Return(int64(1), nil)
		th.Store.EXPECT().GetBlocksWithParentAndType("board_id", "parent_id", "view").Return([]*model.Block{{}}, nil)

		withinLimits, err := th.App.isWithinViewsLimit("board_id", &model.Block{ParentID: "parent_id"})
		assert.NoError(t, err)
		assert.False(t, withinLimits)
	})

	t.Run("view limit already exceeded", func(t *testing.T) {
		th.Store.EXPECT().GetLicense().Return(fakeLicense)

		cloudLimit := &mmModel.ProductLimits{
			Boards: &mmModel.BoardsLimits{
				Views: mmModel.NewInt(2),
			},
		}
		th.Store.EXPECT().GetCloudLimits().Return(cloudLimit, nil)
		th.Store.EXPECT().GetUsedCardsCount().Return(1, nil)
		th.Store.EXPECT().GetCardLimitTimestamp().Return(int64(1), nil)
		th.Store.EXPECT().GetBlocksWithParentAndType("board_id", "parent_id", "view").Return([]*model.Block{{}, {}, {}}, nil)

		withinLimits, err := th.App.isWithinViewsLimit("board_id", &model.Block{ParentID: "parent_id"})
		assert.NoError(t, err)
		assert.False(t, withinLimits)
	})

	t.Run("creating first view", func(t *testing.T) {
		th.Store.EXPECT().GetLicense().Return(fakeLicense)

		cloudLimit := &mmModel.ProductLimits{
			Boards: &mmModel.BoardsLimits{
				Views: mmModel.NewInt(2),
			},
		}
		th.Store.EXPECT().GetCloudLimits().Return(cloudLimit, nil)
		th.Store.EXPECT().GetUsedCardsCount().Return(1, nil)
		th.Store.EXPECT().GetCardLimitTimestamp().Return(int64(1), nil)
		th.Store.EXPECT().GetBlocksWithParentAndType("board_id", "parent_id", "view").Return([]*model.Block{}, nil)

		withinLimits, err := th.App.isWithinViewsLimit("board_id", &model.Block{ParentID: "parent_id"})
		assert.NoError(t, err)
		assert.True(t, withinLimits)
	})

	t.Run("is not a cloud SKU so limits don't apply", func(t *testing.T) {
		nonCloudLicense := &mmModel.License{
			Features: &mmModel.Features{Cloud: mmModel.NewBool(false)},
		}
		th.Store.EXPECT().GetLicense().Return(nonCloudLicense)

		withinLimits, err := th.App.isWithinViewsLimit("board_id", &model.Block{ParentID: "parent_id"})
		assert.NoError(t, err)
		assert.True(t, withinLimits)
	})
}

func TestInsertBlocks(t *testing.T) {
	th, tearDown := SetupTestHelper(t)
	defer tearDown()

	t.Run("success scenario", func(t *testing.T) {
		boardID := testBoardID
		block := &model.Block{BoardID: boardID}
		board := &model.Board{ID: boardID}
		th.Store.EXPECT().GetBoard(boardID).Return(board, nil)
		th.Store.EXPECT().InsertBlock(block, "user-id-1").Return(nil)
		th.Store.EXPECT().GetMembersForBoard(boardID).Return([]*model.BoardMember{}, nil)
		_, err := th.App.InsertBlocks([]*model.Block{block}, "user-id-1")
		require.NoError(t, err)
	})

	t.Run("error scenario", func(t *testing.T) {
		boardID := testBoardID
		block := &model.Block{BoardID: boardID}
		board := &model.Board{ID: boardID}
		th.Store.EXPECT().GetBoard(boardID).Return(board, nil)
		th.Store.EXPECT().InsertBlock(block, "user-id-1").Return(blockError{"error"})
		_, err := th.App.InsertBlocks([]*model.Block{block}, "user-id-1")
		require.Error(t, err, "error")
	})

	t.Run("create view within limits", func(t *testing.T) {
		t.Skipf("The Cloud Limits feature has been disabled")

		boardID := testBoardID
		block := &model.Block{
			Type:     model.TypeView,
			ParentID: "parent_id",
			BoardID:  boardID,
		}
		board := &model.Board{ID: boardID}
		th.Store.EXPECT().GetBoard(boardID).Return(board, nil)
		th.Store.EXPECT().InsertBlock(block, "user-id-1").Return(nil)
		th.Store.EXPECT().GetMembersForBoard(boardID).Return([]*model.BoardMember{}, nil)

		// setting up mocks for limits
		fakeLicense := &mmModel.License{
			Features: &mmModel.Features{Cloud: mmModel.NewBool(true)},
		}
		th.Store.EXPECT().GetLicense().Return(fakeLicense)

		cloudLimit := &mmModel.ProductLimits{
			Boards: &mmModel.BoardsLimits{
				Views: mmModel.NewInt(2),
			},
		}
		th.Store.EXPECT().GetCloudLimits().Return(cloudLimit, nil)
		th.Store.EXPECT().GetUsedCardsCount().Return(1, nil)
		th.Store.EXPECT().GetCardLimitTimestamp().Return(int64(1), nil)
		th.Store.EXPECT().GetBlocksWithParentAndType("test-board-id", "parent_id", "view").Return([]*model.Block{{}}, nil)

		_, err := th.App.InsertBlocks([]*model.Block{block}, "user-id-1")
		require.NoError(t, err)
	})

	t.Run("create view exceeding limits", func(t *testing.T) {
		t.Skipf("The Cloud Limits feature has been disabled")

		boardID := testBoardID
		block := &model.Block{
			Type:     model.TypeView,
			ParentID: "parent_id",
			BoardID:  boardID,
		}
		board := &model.Board{ID: boardID}
		th.Store.EXPECT().GetBoard(boardID).Return(board, nil)

		// setting up mocks for limits
		fakeLicense := &mmModel.License{
			Features: &mmModel.Features{Cloud: mmModel.NewBool(true)},
		}
		th.Store.EXPECT().GetLicense().Return(fakeLicense)

		cloudLimit := &mmModel.ProductLimits{
			Boards: &mmModel.BoardsLimits{
				Views: mmModel.NewInt(2),
			},
		}
		th.Store.EXPECT().GetCloudLimits().Return(cloudLimit, nil)
		th.Store.EXPECT().GetUsedCardsCount().Return(1, nil)
		th.Store.EXPECT().GetCardLimitTimestamp().Return(int64(1), nil)
		th.Store.EXPECT().GetBlocksWithParentAndType("test-board-id", "parent_id", "view").Return([]*model.Block{{}, {}}, nil)

		_, err := th.App.InsertBlocks([]*model.Block{block}, "user-id-1")
		require.Error(t, err)
	})

	t.Run("creating multiple views, reaching limit in the process", func(t *testing.T) {
		t.Skipf("Will be fixed soon")

		boardID := testBoardID
		view1 := &model.Block{
			Type:     model.TypeView,
			ParentID: "parent_id",
			BoardID:  boardID,
		}

		view2 := &model.Block{
			Type:     model.TypeView,
			ParentID: "parent_id",
			BoardID:  boardID,
		}

		board := &model.Board{ID: boardID}
		th.Store.EXPECT().GetBoard(boardID).Return(board, nil)
		th.Store.EXPECT().InsertBlock(view1, "user-id-1").Return(nil).Times(2)
		th.Store.EXPECT().GetMembersForBoard(boardID).Return([]*model.BoardMember{}, nil).Times(2)

		// setting up mocks for limits
		fakeLicense := &mmModel.License{
			Features: &mmModel.Features{Cloud: mmModel.NewBool(true)},
		}
		th.Store.EXPECT().GetLicense().Return(fakeLicense).Times(2)

		cloudLimit := &mmModel.ProductLimits{
			Boards: &mmModel.BoardsLimits{
				Views: mmModel.NewInt(2),
			},
		}
		th.Store.EXPECT().GetCloudLimits().Return(cloudLimit, nil).Times(2)
		th.Store.EXPECT().GetUsedCardsCount().Return(1, nil).Times(2)
		th.Store.EXPECT().GetCardLimitTimestamp().Return(int64(1), nil).Times(2)
		th.Store.EXPECT().GetBlocksWithParentAndType("test-board-id", "parent_id", "view").Return([]*model.Block{{}}, nil).Times(2)

		_, err := th.App.InsertBlocks([]*model.Block{view1, view2}, "user-id-1")
		require.Error(t, err)
	})
}
