package app

import (
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/mattermost/focalboard/server/model"
	"github.com/mattermost/focalboard/server/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateCard(t *testing.T) {
	th, tearDown := SetupTestHelper(t)
	defer tearDown()

	board := &model.Board{
		ID: utils.NewID(utils.IDTypeBoard),
	}
	userID := utils.NewID(utils.IDTypeUser)

	props := makeProps(3)

	card := &model.Card{
		BoardID:      board.ID,
		CreatedBy:    userID,
		ModifiedBy:   userID,
		Title:        "test card",
		TaskID:       "#99",
		ContentOrder: []string{utils.NewID(utils.IDTypeBlock), utils.NewID(utils.IDTypeBlock)},
		Properties:   props,
	}
	block := model.Card2Block(card)

	t.Run("success scenario", func(t *testing.T) {
		existingBlocks := []*model.Block{
			{
				ID:      utils.NewID(utils.IDTypeCard),
				BoardID: board.ID,
				Type:    model.TypeCard,
				Fields: map[string]interface{}{
					"taskId":       "#3",
					"globalTaskId": "G-3",
					"properties": map[string]interface{}{
						cardGlobalTaskIDProperty: "G-3",
					},
				},
			},
		}
		th.Store.EXPECT().GetBlocks(model.QueryBlocksOptions{
			BoardID:   board.ID,
			BlockType: model.TypeCard,
		}).Return(existingBlocks, nil)
		th.Store.EXPECT().GetBlocks(model.QueryBlocksOptions{
			BoardID:   board.ID,
			BlockType: model.TypeCard,
		}).Return(existingBlocks, nil)
		expectCardGlobalTaskIDCounter(th, "3")
		th.Store.EXPECT().GetBoard(board.ID).Return(board, nil)
		th.Store.EXPECT().InsertBlock(gomock.AssignableToTypeOf(reflect.TypeOf(block)), userID).Return(nil)
		th.Store.EXPECT().GetMembersForBoard(board.ID).Return([]*model.BoardMember{}, nil)

		newCard, err := th.App.CreateCard(card, board.ID, userID, false)

		require.NoError(t, err)
		require.Equal(t, card.BoardID, newCard.BoardID)
		require.Equal(t, card.Title, newCard.Title)
		require.Equal(t, card.ContentOrder, newCard.ContentOrder)
		require.Equal(t, "#4", newCard.TaskID)
		require.Equal(t, "G-4", newCard.GlobalTaskID)
		require.Equal(t, "G-4", newCard.Properties[cardGlobalTaskIDProperty])
	})

	t.Run("error scenario", func(t *testing.T) {
		card.TaskID = ""
		th.Store.EXPECT().GetBoard(board.ID).Return(board, nil)
		th.Store.EXPECT().InsertBlock(gomock.AssignableToTypeOf(reflect.TypeOf(block)), userID).Return(blockError{"error"})

		newCard, err := th.App.CreateCard(card, board.ID, userID, false)

		require.Error(t, err, "error")
		require.Nil(t, newCard)
	})
}

func TestCardTaskIDNumber(t *testing.T) {
	t.Run("accepts hash prefixed number", func(t *testing.T) {
		number, ok := cardTaskIDNumber("#42")
		require.True(t, ok)
		require.Equal(t, 42, number)
	})

	t.Run("accepts plain number", func(t *testing.T) {
		number, ok := cardTaskIDNumber("42")
		require.True(t, ok)
		require.Equal(t, 42, number)
	})

	t.Run("rejects internal card ids", func(t *testing.T) {
		_, ok := cardTaskIDNumber(utils.NewID(utils.IDTypeCard))
		require.False(t, ok)
	})

	t.Run("rejects zero", func(t *testing.T) {
		_, ok := cardTaskIDNumber("#0")
		require.False(t, ok)
	})
}

func TestCardGlobalTaskIDNumber(t *testing.T) {
	t.Run("accepts global number", func(t *testing.T) {
		number, ok := cardGlobalTaskIDNumber("G-42")
		require.True(t, ok)
		require.Equal(t, 42, number)
	})

	t.Run("rejects timestamp based number", func(t *testing.T) {
		_, ok := cardGlobalTaskIDNumber("G-1780519393936")
		require.False(t, ok)
	})
}

func TestNextCardGlobalTaskIDResetsTimestampCounter(t *testing.T) {
	th, tearDown := SetupTestHelper(t)
	defer tearDown()

	blocks := []*model.Block{
		{
			ID:   utils.NewID(utils.IDTypeCard),
			Type: model.TypeCard,
			Fields: map[string]interface{}{
				"globalTaskId": "G-2",
			},
		},
		{
			ID:   utils.NewID(utils.IDTypeCard),
			Type: model.TypeCard,
			Fields: map[string]interface{}{
				"globalTaskId": "G-1780519393936",
			},
		},
	}

	th.Store.EXPECT().GetSystemSetting(cardGlobalTaskIDCounterKey).Return("1780519393936", nil)
	th.Store.EXPECT().GetBlocks(model.QueryBlocksOptions{
		BlockType: model.TypeCard,
	}).Return(blocks, nil)
	th.Store.EXPECT().SetSystemSetting(cardGlobalTaskIDCounterKey, "2").Return(nil)
	th.Store.EXPECT().SetSystemSetting(cardGlobalTaskIDCounterKey, "3").Return(nil)

	globalTaskID, err := th.App.nextCardGlobalTaskIDLocked()

	require.NoError(t, err)
	require.Equal(t, "G-3", globalTaskID)
}

func TestEnsureCardTaskIDs(t *testing.T) {
	th, tearDown := SetupTestHelper(t)
	defer tearDown()

	boardID := utils.NewID(utils.IDTypeBoard)
	opts := model.QueryBlocksOptions{
		BoardID:   boardID,
		BlockType: model.TypeCard,
	}

	oldestMissing := &model.Block{
		ID:       utils.NewID(utils.IDTypeCard),
		BoardID:  boardID,
		Type:     model.TypeCard,
		CreateAt: 10,
		Fields:   map[string]interface{}{},
	}
	existing := &model.Block{
		ID:       utils.NewID(utils.IDTypeCard),
		BoardID:  boardID,
		Type:     model.TypeCard,
		CreateAt: 20,
		Fields:   map[string]interface{}{"taskId": "#4"},
	}
	invalid := &model.Block{
		ID:       utils.NewID(utils.IDTypeCard),
		BoardID:  boardID,
		Type:     model.TypeCard,
		CreateAt: 30,
		Fields:   map[string]interface{}{"taskId": utils.NewID(utils.IDTypeCard)},
	}
	duplicate := &model.Block{
		ID:       utils.NewID(utils.IDTypeCard),
		BoardID:  boardID,
		Type:     model.TypeCard,
		CreateAt: 35,
		Fields:   map[string]interface{}{"taskId": "#4"},
	}
	missingFields := &model.Block{
		ID:       utils.NewID(utils.IDTypeCard),
		BoardID:  boardID,
		Type:     model.TypeCard,
		CreateAt: 40,
	}

	th.Store.EXPECT().GetBlocks(opts).Return([]*model.Block{missingFields, invalid, duplicate, existing, oldestMissing}, nil)
	th.Store.EXPECT().PatchBlock(oldestMissing.ID, &model.BlockPatch{
		UpdatedFields: map[string]interface{}{"taskId": "#5"},
	}, model.SystemUserID).Return(nil)
	th.Store.EXPECT().PatchBlock(invalid.ID, &model.BlockPatch{
		UpdatedFields: map[string]interface{}{"taskId": "#6"},
	}, model.SystemUserID).Return(nil)
	th.Store.EXPECT().PatchBlock(duplicate.ID, &model.BlockPatch{
		UpdatedFields: map[string]interface{}{"taskId": "#7"},
	}, model.SystemUserID).Return(nil)
	th.Store.EXPECT().PatchBlock(missingFields.ID, &model.BlockPatch{
		UpdatedFields: map[string]interface{}{"taskId": "#8"},
	}, model.SystemUserID).Return(nil)

	err := th.App.ensureCardTaskIDs(boardID)
	require.NoError(t, err)
	require.Equal(t, "#5", oldestMissing.Fields["taskId"])
	require.Equal(t, "#6", invalid.Fields["taskId"])
	require.Equal(t, "#7", duplicate.Fields["taskId"])
	require.Equal(t, "#8", missingFields.Fields["taskId"])
	require.Equal(t, "#4", existing.Fields["taskId"])
}

func TestCardTaskIDLifecycleUsesUniqueIDs(t *testing.T) {
	th, tearDown := SetupTestHelper(t)
	defer tearDown()

	boardID := utils.NewID(utils.IDTypeBoard)
	userID := utils.NewID(utils.IDTypeUser)
	board := &model.Board{ID: boardID}

	var blocksMux sync.Mutex
	blocks := []*model.Block{
		{
			ID:       utils.NewID(utils.IDTypeCard),
			BoardID:  boardID,
			Type:     model.TypeCard,
			Title:    "old missing task id",
			CreateAt: 10,
			Fields:   map[string]interface{}{},
		},
		{
			ID:       utils.NewID(utils.IDTypeCard),
			BoardID:  boardID,
			Type:     model.TypeCard,
			Title:    "old existing task id",
			CreateAt: 20,
			Fields:   map[string]interface{}{"taskId": "#2", "globalTaskId": "G-2"},
		},
		{
			ID:       utils.NewID(utils.IDTypeCard),
			BoardID:  boardID,
			Type:     model.TypeCard,
			Title:    "old duplicate task id",
			CreateAt: 30,
			Fields:   map[string]interface{}{"taskId": "#2", "globalTaskId": "G-2"},
		},
		{
			ID:       utils.NewID(utils.IDTypeCard),
			BoardID:  boardID,
			Type:     model.TypeCard,
			Title:    "old internal id task id",
			CreateAt: 40,
			Fields:   map[string]interface{}{"taskId": utils.NewID(utils.IDTypeCard), "globalTaskId": utils.NewID(utils.IDTypeCard)},
		},
	}

	cardBlocks := func(boardFilter string) []*model.Block {
		out := make([]*model.Block, 0, len(blocks))
		for _, block := range blocks {
			if (boardFilter == "" || block.BoardID == boardFilter) && block.Type == model.TypeCard {
				out = append(out, cloneBlockForTest(block))
			}
		}
		return out
	}

	th.Store.EXPECT().GetBlocks(gomock.Any()).DoAndReturn(func(opts model.QueryBlocksOptions) ([]*model.Block, error) {
		blocksMux.Lock()
		defer blocksMux.Unlock()
		require.Equal(t, model.BlockType(model.TypeCard), opts.BlockType)
		return cardBlocks(opts.BoardID), nil
	}).AnyTimes()
	th.Store.EXPECT().GetBlocksForBoard(boardID).DoAndReturn(func(string) ([]*model.Block, error) {
		blocksMux.Lock()
		defer blocksMux.Unlock()
		return cardBlocks(boardID), nil
	}).AnyTimes()
	th.Store.EXPECT().PatchBlock(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(blockID string, blockPatch *model.BlockPatch, userID string) error {
			blocksMux.Lock()
			defer blocksMux.Unlock()
			for _, block := range blocks {
				if block.ID == blockID {
					blockPatch.Patch(block)
					return nil
				}
			}
			return model.NewErrNotFound(blockID)
		},
	).AnyTimes()
	th.Store.EXPECT().GetBoard(boardID).Return(board, nil).AnyTimes()
	expectCardGlobalTaskIDCounter(th, "2")
	th.Store.EXPECT().InsertBlock(gomock.Any(), userID).DoAndReturn(func(block *model.Block, _ string) error {
		blocksMux.Lock()
		defer blocksMux.Unlock()
		blocks = append(blocks, cloneBlockForTest(block))
		return nil
	}).AnyTimes()
	th.Store.EXPECT().DuplicateBlock(boardID, gomock.Any(), userID, false).DoAndReturn(
		func(_ string, blockID string, _ string, _ bool) ([]*model.Block, error) {
			blocksMux.Lock()
			defer blocksMux.Unlock()
			for _, block := range blocks {
				if block.ID == blockID {
					duplicate := cloneBlockForTest(block)
					duplicate.ID = utils.NewID(utils.IDTypeCard)
					blocks = append(blocks, duplicate)
					return []*model.Block{cloneBlockForTest(duplicate)}, nil
				}
			}
			return nil, model.NewErrNotFound(blockID)
		},
	)
	th.Store.EXPECT().GetMembersForBoard(boardID).Return([]*model.BoardMember{}, nil).AnyTimes()

	cards, err := th.App.GetCardsForBoard(boardID, 0, 0)
	require.NoError(t, err)
	require.Len(t, cards, 4)
	requireUniqueTaskIDs(t, cards)
	requireTaskIDByTitle(t, cards, "old missing task id", "#3")
	requireTaskIDByTitle(t, cards, "old existing task id", "#2")
	requireTaskIDByTitle(t, cards, "old duplicate task id", "#4")
	requireTaskIDByTitle(t, cards, "old internal id task id", "#5")
	requireUniqueGlobalTaskIDs(t, cards)
	requireGlobalTaskIDByTitle(t, cards, "old missing task id", "G-3")
	requireGlobalTaskIDByTitle(t, cards, "old existing task id", "G-2")
	requireGlobalTaskIDByTitle(t, cards, "old duplicate task id", "G-4")
	requireGlobalTaskIDByTitle(t, cards, "old internal id task id", "G-5")

	allBlocks, err := th.App.GetBlocksForBoard(boardID)
	require.NoError(t, err)
	requireTaskIDByBlockTitle(t, allBlocks, "old missing task id", "#3")
	requireTaskIDByBlockTitle(t, allBlocks, "old existing task id", "#2")
	requireTaskIDByBlockTitle(t, allBlocks, "old duplicate task id", "#4")
	requireTaskIDByBlockTitle(t, allBlocks, "old internal id task id", "#5")
	requireGlobalTaskIDByBlockTitle(t, allBlocks, "old missing task id", "G-3")
	requireGlobalTaskIDByBlockTitle(t, allBlocks, "old existing task id", "G-2")
	requireGlobalTaskIDByBlockTitle(t, allBlocks, "old duplicate task id", "G-4")
	requireGlobalTaskIDByBlockTitle(t, allBlocks, "old internal id task id", "G-5")

	const concurrentCards = 10
	var wg sync.WaitGroup
	errCh := make(chan error, concurrentCards)
	for i := 0; i < concurrentCards; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			clientTaskID := utils.NewID(utils.IDTypeCard)
			newBlocks, cErr := th.App.InsertBlocks([]*model.Block{
				{
					ID:      utils.NewID(utils.IDTypeCard),
					BoardID: boardID,
					Type:    model.TypeCard,
					Title:   fmt.Sprintf("new card %d", i),
					Fields: map[string]interface{}{
						"taskId":       clientTaskID,
						"contentOrder": []string{},
						"properties":   map[string]any{},
					},
				},
			}, userID)
			if cErr == nil {
				serverTaskID, _ := newBlocks[0].Fields["taskId"].(string)
				require.NotEqual(t, clientTaskID, serverTaskID)
				serverGlobalTaskID, _ := newBlocks[0].Fields["globalTaskId"].(string)
				require.NotEmpty(t, serverGlobalTaskID)
				require.Equal(t, serverGlobalTaskID, newBlocks[0].Fields["properties"].(map[string]interface{})[cardGlobalTaskIDProperty])
			}
			errCh <- cErr
		}(i)
	}
	wg.Wait()
	close(errCh)
	for cErr := range errCh {
		require.NoError(t, cErr)
	}

	cards, err = th.App.GetCardsForBoard(boardID, 0, 0)
	require.NoError(t, err)
	require.Len(t, cards, 4+concurrentCards)
	requireUniqueTaskIDs(t, cards)
	requireUniqueGlobalTaskIDs(t, cards)

	_, err = th.App.DuplicateBlock(boardID, cards[0].ID, userID, false)
	require.NoError(t, err)

	cards, err = th.App.GetCardsForBoard(boardID, 0, 0)
	require.NoError(t, err)
	require.Len(t, cards, 5+concurrentCards)
	requireUniqueTaskIDs(t, cards)
	requireUniqueGlobalTaskIDs(t, cards)
	requireTaskIDExists(t, cards, "#16")
	requireGlobalTaskIDExists(t, cards, "G-16")
}

func TestGetCards(t *testing.T) {
	th, tearDown := SetupTestHelper(t)
	defer tearDown()

	board := &model.Board{
		ID: utils.NewID(utils.IDTypeBoard),
	}

	const cardCount = 25

	// make some cards
	blocks := make([]*model.Block, 0, cardCount)
	for i := 0; i < cardCount; i++ {
		card := &model.Block{
			ID:       utils.NewID(utils.IDTypeBlock),
			ParentID: board.ID,
			Schema:   1,
			Type:     model.TypeCard,
			Title:    fmt.Sprintf("card %d", i),
			BoardID:  board.ID,
			Fields: map[string]interface{}{
				"taskId":       fmt.Sprintf("#%d", i+1),
				"globalTaskId": fmt.Sprintf("G-%d", i+1),
				"properties": map[string]interface{}{
					cardGlobalTaskIDProperty: fmt.Sprintf("G-%d", i+1),
				},
			},
		}
		blocks = append(blocks, card)
	}

	t.Run("success scenario", func(t *testing.T) {
		opts := model.QueryBlocksOptions{
			BoardID:   board.ID,
			BlockType: model.TypeCard,
		}

		th.Store.EXPECT().GetBlocks(opts).Return(blocks, nil)
		th.Store.EXPECT().GetBlocks(opts).Return(blocks, nil)
		th.Store.EXPECT().GetBlocks(opts).Return(blocks, nil)

		cards, err := th.App.GetCardsForBoard(board.ID, 0, 0)
		require.NoError(t, err)
		assert.Len(t, cards, cardCount)
	})

	t.Run("error scenario", func(t *testing.T) {
		opts := model.QueryBlocksOptions{
			BoardID:   board.ID,
			BlockType: model.TypeCard,
		}

		th.Store.EXPECT().GetBlocks(opts).Return(nil, blockError{"error"})

		cards, err := th.App.GetCardsForBoard(board.ID, 0, 0)
		require.Error(t, err)
		require.Nil(t, cards)
	})
}

func TestPatchCard(t *testing.T) {
	th, tearDown := SetupTestHelper(t)
	defer tearDown()

	board := &model.Board{
		ID: utils.NewID(utils.IDTypeBoard),
	}
	userID := utils.NewID(utils.IDTypeUser)

	props := makeProps(3)

	card := &model.Card{
		BoardID:      board.ID,
		CreatedBy:    userID,
		ModifiedBy:   userID,
		Title:        "test card for patch",
		ContentOrder: []string{utils.NewID(utils.IDTypeBlock), utils.NewID(utils.IDTypeBlock)},
		Properties:   copyProps(props),
	}

	newTitle := "patched"
	newIcon := "😀"
	newContentOrder := reverse(card.ContentOrder)

	cardPatch := &model.CardPatch{
		Title:             &newTitle,
		ContentOrder:      &newContentOrder,
		Icon:              &newIcon,
		UpdatedProperties: modifyProps(props),
	}

	t.Run("success scenario", func(t *testing.T) {
		expectedPatchedCard := cardPatch.Patch(card)
		expectedPatchedBlock := model.Card2Block(expectedPatchedCard)

		var blockPatch *model.BlockPatch
		th.Store.EXPECT().GetBoard(board.ID).Return(board, nil)
		th.Store.EXPECT().PatchBlock(card.ID, gomock.AssignableToTypeOf(reflect.TypeOf(blockPatch)), userID).Return(nil)
		th.Store.EXPECT().GetMembersForBoard(board.ID).Return([]*model.BoardMember{}, nil)
		th.Store.EXPECT().GetBlock(card.ID).Return(expectedPatchedBlock, nil).AnyTimes()

		patchedCard, err := th.App.PatchCard(cardPatch, card.ID, userID, false)

		require.NoError(t, err)
		require.Equal(t, board.ID, patchedCard.BoardID)
		require.Equal(t, newTitle, patchedCard.Title)
		require.Equal(t, newIcon, patchedCard.Icon)
		require.Equal(t, newContentOrder, patchedCard.ContentOrder)
		require.EqualValues(t, expectedPatchedCard.Properties, patchedCard.Properties)
	})

	t.Run("error scenario", func(t *testing.T) {
		var blockPatch *model.BlockPatch
		th.Store.EXPECT().GetBoard(board.ID).Return(board, nil)
		th.Store.EXPECT().PatchBlock(card.ID, gomock.AssignableToTypeOf(reflect.TypeOf(blockPatch)), userID).Return(blockError{"error"})

		patchedCard, err := th.App.PatchCard(cardPatch, card.ID, userID, false)

		require.Error(t, err, "error")
		require.Nil(t, patchedCard)
	})
}

func TestGetCard(t *testing.T) {
	th, tearDown := SetupTestHelper(t)
	defer tearDown()

	boardID := utils.NewID(utils.IDTypeBoard)
	userID := utils.NewID(utils.IDTypeUser)
	props := makeProps(5)
	contentOrder := []string{utils.NewID(utils.IDTypeUser), utils.NewID(utils.IDTypeUser)}
	fields := make(map[string]any)
	fields["contentOrder"] = contentOrder
	fields["properties"] = props
	fields["icon"] = "😀"
	fields["isTemplate"] = true

	block := &model.Block{
		ID:         utils.NewID(utils.IDTypeBlock),
		ParentID:   boardID,
		Type:       model.TypeCard,
		Title:      "test card",
		BoardID:    boardID,
		Fields:     fields,
		CreatedBy:  userID,
		ModifiedBy: userID,
	}

	t.Run("success scenario", func(t *testing.T) {
		th.Store.EXPECT().GetBlock(block.ID).Return(block, nil)

		card, err := th.App.GetCardByID(block.ID)

		require.NoError(t, err)
		require.Equal(t, boardID, card.BoardID)
		require.Equal(t, block.Title, card.Title)
		require.Equal(t, "😀", card.Icon)
		require.Equal(t, true, card.IsTemplate)
		require.Equal(t, contentOrder, card.ContentOrder)
		require.EqualValues(t, props, card.Properties)
	})

	t.Run("not found", func(t *testing.T) {
		bogusID := utils.NewID(utils.IDTypeBlock)
		th.Store.EXPECT().GetBlock(bogusID).Return(nil, model.NewErrNotFound(bogusID))

		card, err := th.App.GetCardByID(bogusID)

		require.Error(t, err, "error")
		require.True(t, model.IsErrNotFound(err))
		require.Nil(t, card)
	})

	t.Run("error scenario", func(t *testing.T) {
		th.Store.EXPECT().GetBlock(block.ID).Return(nil, blockError{"error"})

		card, err := th.App.GetCardByID(block.ID)

		require.Error(t, err, "error")
		require.Nil(t, card)
	})
}

// reverse is a helper function to copy and reverse a slice of strings.
func reverse(src []string) []string {
	out := make([]string, 0, len(src))
	for i := len(src) - 1; i >= 0; i-- {
		out = append(out, src[i])
	}
	return out
}

func makeProps(count int) map[string]any {
	props := make(map[string]any)
	for i := 0; i < count; i++ {
		props[utils.NewID(utils.IDTypeBlock)] = utils.NewID(utils.IDTypeBlock)
	}
	return props
}

func copyProps(m map[string]any) map[string]any {
	out := make(map[string]any)
	for k, v := range m {
		out[k] = v
	}
	return out
}

func modifyProps(m map[string]any) map[string]any {
	out := make(map[string]any)
	for k := range m {
		out[k] = utils.NewID(utils.IDTypeBlock)
	}
	return out
}

func cloneBlockForTest(block *model.Block) *model.Block {
	clone := *block
	if block.Fields != nil {
		clone.Fields = make(map[string]interface{}, len(block.Fields))
		for key, value := range block.Fields {
			if props, ok := value.(map[string]interface{}); ok {
				propsClone := make(map[string]interface{}, len(props))
				for propKey, propValue := range props {
					propsClone[propKey] = propValue
				}
				clone.Fields[key] = propsClone
				continue
			}
			clone.Fields[key] = value
		}
	}
	return &clone
}

func requireUniqueTaskIDs(t *testing.T, cards []*model.Card) {
	t.Helper()
	seen := make(map[string]struct{}, len(cards))
	for _, card := range cards {
		require.NotEmpty(t, card.TaskID, card.Title)
		_, exists := seen[card.TaskID]
		require.False(t, exists, "duplicate task id %s", card.TaskID)
		seen[card.TaskID] = struct{}{}
	}
}

func requireUniqueGlobalTaskIDs(t *testing.T, cards []*model.Card) {
	t.Helper()
	seen := make(map[string]struct{}, len(cards))
	for _, card := range cards {
		require.NotEmpty(t, card.GlobalTaskID, card.Title)
		_, exists := seen[card.GlobalTaskID]
		require.False(t, exists, "duplicate global task id %s", card.GlobalTaskID)
		seen[card.GlobalTaskID] = struct{}{}
		require.Equal(t, card.GlobalTaskID, card.Properties[cardGlobalTaskIDProperty])
	}
}

func requireTaskIDByTitle(t *testing.T, cards []*model.Card, title string, taskID string) {
	t.Helper()
	for _, card := range cards {
		if card.Title == title {
			require.Equal(t, taskID, card.TaskID)
			return
		}
	}
	require.Fail(t, "card title not found", title)
}

func requireGlobalTaskIDByTitle(t *testing.T, cards []*model.Card, title string, globalTaskID string) {
	t.Helper()
	for _, card := range cards {
		if card.Title == title {
			require.Equal(t, globalTaskID, card.GlobalTaskID)
			require.Equal(t, globalTaskID, card.Properties[cardGlobalTaskIDProperty])
			return
		}
	}
	require.Fail(t, "card title not found", title)
}

func requireTaskIDExists(t *testing.T, cards []*model.Card, taskID string) {
	t.Helper()
	for _, card := range cards {
		if card.TaskID == taskID {
			return
		}
	}
	require.Fail(t, "task id not found", taskID)
}

func requireGlobalTaskIDExists(t *testing.T, cards []*model.Card, globalTaskID string) {
	t.Helper()
	for _, card := range cards {
		if card.GlobalTaskID == globalTaskID {
			return
		}
	}
	require.Fail(t, "global task id not found", globalTaskID)
}

func requireTaskIDByBlockTitle(t *testing.T, blocks []*model.Block, title string, taskID string) {
	t.Helper()
	for _, block := range blocks {
		if block.Title == title {
			require.Equal(t, taskID, block.Fields["taskId"])
			return
		}
	}
	require.Fail(t, "block title not found", title)
}

func requireGlobalTaskIDByBlockTitle(t *testing.T, blocks []*model.Block, title string, globalTaskID string) {
	t.Helper()
	for _, block := range blocks {
		if block.Title == title {
			require.Equal(t, globalTaskID, block.Fields["globalTaskId"])
			require.Equal(t, globalTaskID, block.Fields["properties"].(map[string]interface{})[cardGlobalTaskIDProperty])
			return
		}
	}
	require.Fail(t, "block title not found", title)
}

func expectCardGlobalTaskIDCounter(th *TestHelper, initialValue string) {
	counterValue := initialValue
	th.Store.EXPECT().GetSystemSetting(cardGlobalTaskIDCounterKey).DoAndReturn(
		func(string) (string, error) {
			return counterValue, nil
		},
	).AnyTimes()
	th.Store.EXPECT().SetSystemSetting(cardGlobalTaskIDCounterKey, gomock.Any()).DoAndReturn(
		func(_ string, value string) error {
			counterValue = value
			return nil
		},
	).AnyTimes()
}
