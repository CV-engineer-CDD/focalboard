package app

import (
	"fmt"
	"reflect"
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
				Fields:  map[string]interface{}{"taskId": "#3"},
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
		th.Store.EXPECT().GetBoard(board.ID).Return(board, nil)
		th.Store.EXPECT().InsertBlock(gomock.AssignableToTypeOf(reflect.TypeOf(block)), userID).Return(nil)
		th.Store.EXPECT().GetMembersForBoard(board.ID).Return([]*model.BoardMember{}, nil)

		newCard, err := th.App.CreateCard(card, board.ID, userID, false)

		require.NoError(t, err)
		require.Equal(t, card.BoardID, newCard.BoardID)
		require.Equal(t, card.Title, newCard.Title)
		require.Equal(t, card.ContentOrder, newCard.ContentOrder)
		require.EqualValues(t, card.Properties, newCard.Properties)
		require.Equal(t, "#4", newCard.TaskID)
	})

	t.Run("error scenario", func(t *testing.T) {
		card.TaskID = ""
		th.Store.EXPECT().GetBlocks(model.QueryBlocksOptions{
			BoardID:   board.ID,
			BlockType: model.TypeCard,
		}).Return(nil, nil)
		th.Store.EXPECT().GetBlocks(model.QueryBlocksOptions{
			BoardID:   board.ID,
			BlockType: model.TypeCard,
		}).Return(nil, nil)
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
			Fields:   map[string]interface{}{"taskId": fmt.Sprintf("#%d", i+1)},
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
