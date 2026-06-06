// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package app

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/mattermost/focalboard/server/model"
	"github.com/mattermost/focalboard/server/utils"
)

const (
	cardTaskIDPrefix           = "#"
	cardTaskIDCounterKeyPrefix = "focalboard_card_task_id_max_"
	cardGlobalTaskIDPrefix     = "G-"
	cardGlobalTaskIDProperty   = "__globalTaskId"
	cardGlobalTaskIDCounterKey = "focalboard_card_global_task_id_max"
	// Previous builds used a millisecond timestamp as the first global counter.
	// Treat those values as invalid user-facing IDs and compact them back to G-N.
	cardGlobalTaskIDTimestampFloor = 1000000000000
)

func (a *App) CreateCard(card *model.Card, boardID string, userID string, disableNotify bool) (*model.Card, error) {
	// Convert the card struct to a block and insert the block.
	now := utils.GetMillis()

	card.ID = utils.NewID(utils.IDTypeCard)
	card.BoardID = boardID
	card.CreatedBy = userID
	card.ModifiedBy = userID
	card.CreateAt = now
	card.UpdateAt = now
	card.DeleteAt = 0

	block := model.Card2Block(card)

	newBlocks, err := a.InsertBlocksAndNotify([]*model.Block{block}, userID, disableNotify)
	if err != nil {
		return nil, fmt.Errorf("cannot create card: %w", err)
	}

	newCard, err := model.Block2Card(newBlocks[0])
	if err != nil {
		return nil, err
	}

	return newCard, nil
}

func (a *App) prepareCardIDsForInsertLocked(boardID string, block *model.Block) error {
	if err := a.ensureCardTaskIDsLocked(boardID); err != nil {
		return fmt.Errorf("cannot backfill card task ids: %w", err)
	}
	if err := a.ensureCardGlobalTaskIDsForBoardLocked(boardID); err != nil {
		return fmt.Errorf("cannot backfill global card task ids: %w", err)
	}
	taskID, err := a.nextCardTaskIDLocked(boardID)
	if err != nil {
		return fmt.Errorf("cannot create card task id: %w", err)
	}
	globalTaskID, err := a.nextCardGlobalTaskIDLocked()
	if err != nil {
		return fmt.Errorf("cannot create global card task id: %w", err)
	}
	if block.Fields == nil {
		block.Fields = make(map[string]interface{})
	}
	block.Fields["taskId"] = taskID
	block.Fields["globalTaskId"] = globalTaskID
	setCardGlobalTaskIDProperty(block, globalTaskID)
	return nil
}

func (a *App) nextCardTaskID(boardID string) (string, error) {
	unlock := a.lockCardTaskIDs(boardID)
	defer unlock()

	return a.nextCardTaskIDLocked(boardID)
}

func (a *App) nextCardTaskIDLocked(boardID string) (string, error) {
	a.cardTaskIDStateMux.Lock()
	if maxNumber, ok := a.cardTaskIDMaxByBoard[boardID]; ok {
		maxNumber++
		a.cardTaskIDMaxByBoard[boardID] = maxNumber
		a.cardTaskIDStateMux.Unlock()
		if err := a.store.SetSystemSetting(cardTaskIDCounterKey(boardID), strconv.Itoa(maxNumber)); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s%d", cardTaskIDPrefix, maxNumber), nil
	}
	a.cardTaskIDStateMux.Unlock()

	maxNumber, err := a.loadCardTaskIDMaxForBoardLocked(boardID)
	if err != nil {
		return "", err
	}
	counterValue, err := a.store.GetSystemSetting(cardTaskIDCounterKey(boardID))
	if err != nil {
		return "", err
	}
	if counterValue != "" {
		counter, cErr := strconv.Atoi(counterValue)
		if cErr != nil {
			return "", cErr
		}
		if counter > maxNumber {
			maxNumber = counter
		}
	}

	maxNumber++
	a.cardTaskIDStateMux.Lock()
	a.cardTaskIDMaxByBoard[boardID] = maxNumber
	a.cardTaskIDStateMux.Unlock()
	if err := a.store.SetSystemSetting(cardTaskIDCounterKey(boardID), strconv.Itoa(maxNumber)); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%d", cardTaskIDPrefix, maxNumber), nil
}

func (a *App) nextCardGlobalTaskIDLocked() (string, error) {
	if !a.cardGlobalTaskIDCounterLoaded {
		loadedMaxFromBlocks := false
		counterValue, err := a.store.GetSystemSetting(cardGlobalTaskIDCounterKey)
		if err != nil {
			return "", err
		}

		if counterValue != "" {
			counter, cErr := strconv.Atoi(counterValue)
			if cErr != nil {
				return "", cErr
			}
			a.cardGlobalTaskIDMax = counter
		} else {
			counter, cErr := a.loadCardGlobalTaskIDMaxLocked()
			if cErr != nil {
				return "", cErr
			}
			a.cardGlobalTaskIDMax = counter
			loadedMaxFromBlocks = true
		}
		if isTimestampCardGlobalTaskIDNumber(a.cardGlobalTaskIDMax) {
			counter, cErr := a.loadCardGlobalTaskIDMaxLocked()
			if cErr != nil {
				return "", cErr
			}
			a.cardGlobalTaskIDMax = counter
			loadedMaxFromBlocks = true
		}
		if !loadedMaxFromBlocks {
			counter, cErr := a.loadCardGlobalTaskIDMaxLocked()
			if cErr != nil {
				return "", cErr
			}
			if counter > a.cardGlobalTaskIDMax {
				a.cardGlobalTaskIDMax = counter
			}
		}
		a.cardGlobalTaskIDCounterLoaded = true
	}

	a.cardGlobalTaskIDMax++
	if err := a.store.SetSystemSetting(cardGlobalTaskIDCounterKey, strconv.Itoa(a.cardGlobalTaskIDMax)); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%d", cardGlobalTaskIDPrefix, a.cardGlobalTaskIDMax), nil
}

func (a *App) refreshCardIDCountersAfterDelete(block *model.Block) error {
	if block == nil || block.Type != model.TypeCard {
		return nil
	}

	unlock := a.lockCardTaskIDs(block.BoardID)
	defer unlock()
	a.cardGlobalTaskIDMux.Lock()
	defer a.cardGlobalTaskIDMux.Unlock()

	if block.Fields != nil {
		taskID, _ := block.Fields["taskId"].(string)
		if err := a.refreshCardTaskIDCounterAfterDeleteLocked(block.BoardID, taskID); err != nil {
			return err
		}

		globalTaskID, _ := block.Fields["globalTaskId"].(string)
		if err := a.refreshCardGlobalTaskIDCounterAfterDeleteLocked(globalTaskID); err != nil {
			return err
		}
	}

	return nil
}

func (a *App) refreshCardTaskIDCounterAfterDeleteLocked(boardID string, taskID string) error {
	deletedNumber, ok := cardTaskIDNumber(taskID)
	if !ok {
		return nil
	}

	a.cardTaskIDStateMux.Lock()
	currentMax, loaded := a.cardTaskIDMaxByBoard[boardID]
	a.cardTaskIDStateMux.Unlock()
	if !loaded || deletedNumber < currentMax {
		return nil
	}

	maxNumber, err := a.loadCardTaskIDMaxForBoardLocked(boardID)
	if err != nil {
		return err
	}

	a.cardTaskIDStateMux.Lock()
	a.cardTaskIDMaxByBoard[boardID] = maxNumber
	a.cardTaskIDBackfilledBoards[boardID] = true
	a.cardTaskIDStateMux.Unlock()
	if err := a.store.SetSystemSetting(cardTaskIDCounterKey(boardID), strconv.Itoa(maxNumber)); err != nil {
		return err
	}
	return nil
}

func (a *App) refreshCardGlobalTaskIDCounterAfterDeleteLocked(globalTaskID string) error {
	deletedNumber, ok := cardGlobalTaskIDNumber(globalTaskID)
	if !ok {
		return nil
	}

	currentMax := a.cardGlobalTaskIDMax
	if !a.cardGlobalTaskIDCounterLoaded {
		counterValue, err := a.store.GetSystemSetting(cardGlobalTaskIDCounterKey)
		if err != nil {
			return err
		}
		if counterValue == "" {
			return nil
		}
		counter, cErr := strconv.Atoi(counterValue)
		if cErr != nil {
			return cErr
		}
		currentMax = counter
	}
	if !isTimestampCardGlobalTaskIDNumber(currentMax) && deletedNumber < currentMax {
		return nil
	}

	maxNumber, err := a.loadCardGlobalTaskIDMaxLocked()
	if err != nil {
		return err
	}
	a.cardGlobalTaskIDMax = maxNumber
	a.cardGlobalTaskIDCounterLoaded = true
	return nil
}

func (a *App) loadCardTaskIDMaxForBoardLocked(boardID string) (int, error) {
	opts := model.QueryBlocksOptions{
		BoardID:   boardID,
		BlockType: model.TypeCard,
	}

	blocks, err := a.store.GetBlocks(opts)
	if err != nil {
		return 0, err
	}
	deletedBlocks, err := a.store.GetDeletedBlocksForBoard(boardID)
	if err != nil {
		return 0, err
	}
	blocks = append(blocks, deletedBlocks...)

	maxNumber := 0
	for _, block := range blocks {
		if !isCardBlock(block) || block.Fields == nil {
			continue
		}

		taskID, _ := block.Fields["taskId"].(string)
		if number, ok := cardTaskIDNumber(taskID); ok && number > maxNumber {
			maxNumber = number
		}
	}
	return maxNumber, nil
}

func (a *App) reconcileUndeletedCardIDs(block *model.Block, modifiedBy string) error {
	if !isActiveCardBlock(block) {
		return nil
	}

	unlock := a.lockCardTaskIDs(block.BoardID)
	defer unlock()
	a.cardGlobalTaskIDMux.Lock()
	defer a.cardGlobalTaskIDMux.Unlock()

	if block.Fields == nil {
		block.Fields = make(map[string]interface{})
	}

	updatedFields := make(map[string]interface{})
	taskMax, usedTaskIDs, err := a.activeCardTaskIDUsageForBoardLocked(block.BoardID, block.ID)
	if err != nil {
		return err
	}
	taskID, _ := block.Fields["taskId"].(string)
	taskNumber, taskOK := cardTaskIDNumber(taskID)
	_, taskConflict := usedTaskIDs[taskNumber]
	if !taskOK || taskConflict {
		taskMax++
		taskID = fmt.Sprintf("%s%d", cardTaskIDPrefix, taskMax)
		block.Fields["taskId"] = taskID
		updatedFields["taskId"] = taskID
	} else if taskNumber > taskMax {
		taskMax = taskNumber
	}

	globalMax, usedGlobalTaskIDs, err := a.activeCardGlobalTaskIDUsageLocked(block.ID)
	if err != nil {
		return err
	}
	globalTaskID, _ := block.Fields["globalTaskId"].(string)
	globalNumber, globalOK := cardGlobalTaskIDNumber(globalTaskID)
	_, globalConflict := usedGlobalTaskIDs[globalNumber]
	if !globalOK || globalConflict {
		globalMax++
		globalTaskID = fmt.Sprintf("%s%d", cardGlobalTaskIDPrefix, globalMax)
		block.Fields["globalTaskId"] = globalTaskID
		updatedFields["globalTaskId"] = globalTaskID
		setCardGlobalTaskIDProperty(block, globalTaskID)
		updatedFields["properties"] = block.Fields["properties"]
	} else {
		if globalNumber > globalMax {
			globalMax = globalNumber
		}
		if setCardGlobalTaskIDProperty(block, globalTaskID) {
			updatedFields["properties"] = block.Fields["properties"]
		}
	}

	a.cardTaskIDStateMux.Lock()
	a.cardTaskIDMaxByBoard[block.BoardID] = taskMax
	a.cardTaskIDBackfilledBoards[block.BoardID] = true
	a.cardTaskIDStateMux.Unlock()
	a.cardGlobalTaskIDMax = globalMax
	a.cardGlobalTaskIDCounterLoaded = true
	if err := a.store.SetSystemSetting(cardGlobalTaskIDCounterKey, strconv.Itoa(globalMax)); err != nil {
		return err
	}

	if len(updatedFields) == 0 {
		return nil
	}
	return a.store.PatchBlock(block.ID, &model.BlockPatch{UpdatedFields: updatedFields}, modifiedBy)
}

func (a *App) reserveCardIDCountersBeforePurge(block *model.Block) error {
	if !isCardBlock(block) || block.Fields == nil {
		return nil
	}

	unlock := a.lockCardTaskIDs(block.BoardID)
	defer unlock()
	a.cardGlobalTaskIDMux.Lock()
	defer a.cardGlobalTaskIDMux.Unlock()

	taskID, _ := block.Fields["taskId"].(string)
	if taskNumber, ok := cardTaskIDNumber(taskID); ok {
		a.cardTaskIDStateMux.Lock()
		currentMax := a.cardTaskIDMaxByBoard[block.BoardID]
		if taskNumber > currentMax {
			a.cardTaskIDMaxByBoard[block.BoardID] = taskNumber
			currentMax = taskNumber
		}
		a.cardTaskIDBackfilledBoards[block.BoardID] = true
		a.cardTaskIDStateMux.Unlock()
		if err := a.store.SetSystemSetting(cardTaskIDCounterKey(block.BoardID), strconv.Itoa(currentMax)); err != nil {
			return err
		}
	}

	globalTaskID, _ := block.Fields["globalTaskId"].(string)
	if globalNumber, ok := cardGlobalTaskIDNumber(globalTaskID); ok {
		if !a.cardGlobalTaskIDCounterLoaded {
			counterValue, err := a.store.GetSystemSetting(cardGlobalTaskIDCounterKey)
			if err != nil {
				return err
			}
			if counterValue != "" {
				counter, cErr := strconv.Atoi(counterValue)
				if cErr != nil {
					return cErr
				}
				a.cardGlobalTaskIDMax = counter
			}
			a.cardGlobalTaskIDCounterLoaded = true
		}
		if globalNumber > a.cardGlobalTaskIDMax {
			a.cardGlobalTaskIDMax = globalNumber
			if err := a.store.SetSystemSetting(cardGlobalTaskIDCounterKey, strconv.Itoa(globalNumber)); err != nil {
				return err
			}
		}
	}

	return nil
}

func (a *App) activeCardTaskIDUsageForBoardLocked(boardID string, excludedBlockID string) (int, map[int]struct{}, error) {
	opts := model.QueryBlocksOptions{
		BoardID:   boardID,
		BlockType: model.TypeCard,
	}

	blocks, err := a.store.GetBlocks(opts)
	if err != nil {
		return 0, nil, err
	}

	maxNumber := 0
	usedNumbers := make(map[int]struct{})
	for _, block := range blocks {
		if !isActiveCardBlock(block) || block.ID == excludedBlockID || block.Fields == nil {
			continue
		}

		taskID, _ := block.Fields["taskId"].(string)
		if number, ok := cardTaskIDNumber(taskID); ok {
			usedNumbers[number] = struct{}{}
			if number > maxNumber {
				maxNumber = number
			}
		}
	}
	return maxNumber, usedNumbers, nil
}

func (a *App) activeCardGlobalTaskIDUsageLocked(excludedBlockID string) (int, map[int]struct{}, error) {
	opts := model.QueryBlocksOptions{
		BlockType: model.TypeCard,
	}

	blocks, err := a.store.GetBlocks(opts)
	if err != nil {
		return 0, nil, err
	}

	maxNumber := 0
	usedNumbers := make(map[int]struct{})
	for _, block := range blocks {
		if !isActiveCardBlock(block) || block.ID == excludedBlockID || block.Fields == nil {
			continue
		}

		globalTaskID, _ := block.Fields["globalTaskId"].(string)
		if number, ok := cardGlobalTaskIDNumber(globalTaskID); ok {
			usedNumbers[number] = struct{}{}
			if number > maxNumber {
				maxNumber = number
			}
		}
	}
	return maxNumber, usedNumbers, nil
}

func (a *App) loadCardGlobalTaskIDMaxLocked() (int, error) {
	opts := model.QueryBlocksOptions{
		BlockType: model.TypeCard,
	}

	blocks, err := a.store.GetBlocks(opts)
	if err != nil {
		return 0, err
	}
	deletedBlocks, err := a.store.GetDeletedBlocksWithType(string(model.TypeCard))
	if err != nil {
		return 0, err
	}
	blocks = append(blocks, deletedBlocks...)

	maxNumber := 0
	for _, block := range blocks {
		if !isCardBlock(block) || block.Fields == nil {
			continue
		}

		globalTaskID, _ := block.Fields["globalTaskId"].(string)
		if number, ok := cardGlobalTaskIDNumber(globalTaskID); ok && number > maxNumber {
			maxNumber = number
		}
	}

	if err := a.store.SetSystemSetting(cardGlobalTaskIDCounterKey, strconv.Itoa(maxNumber)); err != nil {
		return 0, err
	}
	return maxNumber, nil
}

func (a *App) ensureCardTaskIDs(boardID string) error {
	unlock := a.lockCardTaskIDs(boardID)
	defer unlock()

	return a.ensureCardTaskIDsLocked(boardID)
}

func (a *App) ensureCardTaskIDsLocked(boardID string) error {
	a.cardTaskIDStateMux.Lock()
	if a.cardTaskIDBackfilledBoards[boardID] {
		a.cardTaskIDStateMux.Unlock()
		return nil
	}
	a.cardTaskIDStateMux.Unlock()

	opts := model.QueryBlocksOptions{
		BoardID:   boardID,
		BlockType: model.TypeCard,
	}

	blocks, err := a.store.GetBlocks(opts)
	if err != nil {
		return err
	}

	return a.ensureCardTaskIDsForBlocksLocked(boardID, blocks)
}

func (a *App) ensureCardTaskIDsForBlocksLocked(boardID string, blocks []*model.Block) error {
	a.cardTaskIDStateMux.Lock()
	if a.cardTaskIDBackfilledBoards[boardID] {
		a.cardTaskIDStateMux.Unlock()
		return nil
	}
	a.cardTaskIDStateMux.Unlock()

	allCardBlocks := make([]*model.Block, 0, len(blocks))
	activeCardBlocks := make([]*model.Block, 0, len(blocks))
	for _, block := range blocks {
		if !isCardBlock(block) {
			continue
		}
		allCardBlocks = append(allCardBlocks, block)
		if isActiveCardBlock(block) {
			activeCardBlocks = append(activeCardBlocks, block)
		}
	}

	sort.SliceStable(allCardBlocks, func(i, j int) bool {
		left := allCardBlocks[i]
		right := allCardBlocks[j]
		if left.CreateAt != right.CreateAt {
			return left.CreateAt < right.CreateAt
		}
		return left.ID < right.ID
	})
	sort.SliceStable(activeCardBlocks, func(i, j int) bool {
		left := activeCardBlocks[i]
		right := activeCardBlocks[j]
		if left.CreateAt != right.CreateAt {
			return left.CreateAt < right.CreateAt
		}
		return left.ID < right.ID
	})

	maxNumber := 0
	usedNumbers := make(map[int]struct{})
	for _, block := range allCardBlocks {
		if block.Fields == nil {
			continue
		}
		taskID, _ := block.Fields["taskId"].(string)
		if number, ok := cardTaskIDNumber(taskID); ok && number > maxNumber {
			maxNumber = number
		}
	}

	missingBlocks := make([]*model.Block, 0)
	for _, block := range activeCardBlocks {
		taskID, _ := block.Fields["taskId"].(string)
		if number, ok := cardTaskIDNumber(taskID); ok {
			if _, exists := usedNumbers[number]; !exists {
				usedNumbers[number] = struct{}{}
				continue
			}

			block.Fields["taskId"] = ""
			missingBlocks = append(missingBlocks, block)
			continue
		}

		missingBlocks = append(missingBlocks, block)
	}

	for _, block := range missingBlocks {
		maxNumber++
		taskID := fmt.Sprintf("%s%d", cardTaskIDPrefix, maxNumber)
		if block.Fields == nil {
			block.Fields = make(map[string]interface{})
		}
		block.Fields["taskId"] = taskID
		blockPatch := &model.BlockPatch{
			UpdatedFields: map[string]interface{}{
				"taskId": taskID,
			},
		}
		if err := a.store.PatchBlock(block.ID, blockPatch, model.SystemUserID); err != nil {
			return err
		}
	}

	a.cardTaskIDStateMux.Lock()
	a.cardTaskIDMaxByBoard[boardID] = maxNumber
	a.cardTaskIDBackfilledBoards[boardID] = true
	a.cardTaskIDStateMux.Unlock()
	return nil
}

func (a *App) ensureCardGlobalTaskIDsForBoard(boardID string) error {
	a.cardGlobalTaskIDMux.Lock()
	defer a.cardGlobalTaskIDMux.Unlock()

	return a.ensureCardGlobalTaskIDsForBoardLocked(boardID)
}

func (a *App) ensureCardGlobalTaskIDsForBoardLocked(boardID string) error {
	if a.cardGlobalTaskIDBackfilledBoards[boardID] {
		return nil
	}

	opts := model.QueryBlocksOptions{
		BoardID:   boardID,
		BlockType: model.TypeCard,
	}

	blocks, err := a.store.GetBlocks(opts)
	if err != nil {
		return err
	}

	return a.ensureCardGlobalTaskIDsForBlocksLocked(boardID, blocks)
}

func (a *App) ensureCardGlobalTaskIDsForBlocksLocked(boardID string, blocks []*model.Block) error {
	if a.cardGlobalTaskIDBackfilledBoards[boardID] {
		return nil
	}

	cardBlocks := make([]*model.Block, 0, len(blocks))
	for _, block := range blocks {
		if !isActiveCardBlock(block) {
			continue
		}
		cardBlocks = append(cardBlocks, block)
	}

	sort.SliceStable(cardBlocks, func(i, j int) bool {
		left := cardBlocks[i]
		right := cardBlocks[j]
		if left.CreateAt != right.CreateAt {
			return left.CreateAt < right.CreateAt
		}
		return left.ID < right.ID
	})

	usedNumbers := make(map[int]struct{})
	missingBlocks := make([]*model.Block, 0)
	for _, block := range cardBlocks {
		globalTaskID, _ := block.Fields["globalTaskId"].(string)
		if number, ok := cardGlobalTaskIDNumber(globalTaskID); ok {
			if _, exists := usedNumbers[number]; !exists {
				usedNumbers[number] = struct{}{}
				if setCardGlobalTaskIDProperty(block, globalTaskID) {
					if err := a.patchCardGlobalTaskID(block, globalTaskID); err != nil {
						return err
					}
				}
				continue
			}

			block.Fields["globalTaskId"] = ""
			missingBlocks = append(missingBlocks, block)
			continue
		}

		missingBlocks = append(missingBlocks, block)
	}

	for _, block := range missingBlocks {
		globalTaskID, err := a.nextCardGlobalTaskIDLocked()
		if err != nil {
			return err
		}
		if block.Fields == nil {
			block.Fields = make(map[string]interface{})
		}
		block.Fields["globalTaskId"] = globalTaskID
		setCardGlobalTaskIDProperty(block, globalTaskID)
		if err := a.patchCardGlobalTaskID(block, globalTaskID); err != nil {
			return err
		}
	}

	a.cardGlobalTaskIDBackfilledBoards[boardID] = true
	return nil
}

func (a *App) patchCardGlobalTaskID(block *model.Block, globalTaskID string) error {
	props, _ := block.Fields["properties"].(map[string]interface{})
	if props == nil {
		props = make(map[string]interface{})
	}
	props[cardGlobalTaskIDProperty] = globalTaskID

	blockPatch := &model.BlockPatch{
		UpdatedFields: map[string]interface{}{
			"globalTaskId": globalTaskID,
			"properties":   props,
		},
	}
	return a.store.PatchBlock(block.ID, blockPatch, model.SystemUserID)
}

func (a *App) lockCardTaskIDs(boardID string) func() {
	a.cardTaskIDMux.Lock()
	if a.cardTaskIDBoardMux == nil {
		a.cardTaskIDBoardMux = make(map[string]*sync.Mutex)
	}
	boardMux := a.cardTaskIDBoardMux[boardID]
	if boardMux == nil {
		boardMux = &sync.Mutex{}
		a.cardTaskIDBoardMux[boardID] = boardMux
	}
	a.cardTaskIDMux.Unlock()

	boardMux.Lock()
	return boardMux.Unlock
}

func cardTaskIDNumber(taskID string) (int, bool) {
	trimmed := strings.TrimSpace(taskID)
	trimmed = strings.TrimPrefix(trimmed, cardTaskIDPrefix)
	number, err := strconv.Atoi(trimmed)
	if err != nil || number <= 0 {
		return 0, false
	}
	return number, true
}

func cardGlobalTaskIDNumber(globalTaskID string) (int, bool) {
	trimmed := strings.TrimSpace(globalTaskID)
	trimmed = strings.TrimPrefix(trimmed, cardGlobalTaskIDPrefix)
	number, err := strconv.Atoi(trimmed)
	if err != nil || number <= 0 || isTimestampCardGlobalTaskIDNumber(number) {
		return 0, false
	}
	return number, true
}

func isTimestampCardGlobalTaskIDNumber(number int) bool {
	return number >= cardGlobalTaskIDTimestampFloor
}

func isActiveCardBlock(block *model.Block) bool {
	return block != nil && block.Type == model.TypeCard && block.DeleteAt == 0
}

func isCardBlock(block *model.Block) bool {
	return block != nil && block.Type == model.TypeCard
}

func cardTaskIDCounterKey(boardID string) string {
	return cardTaskIDCounterKeyPrefix + boardID
}

func setCardGlobalTaskIDProperty(block *model.Block, globalTaskID string) bool {
	if block.Fields == nil {
		block.Fields = make(map[string]interface{})
	}
	props, _ := block.Fields["properties"].(map[string]interface{})
	if props == nil {
		props = make(map[string]interface{})
		block.Fields["properties"] = props
	}
	if props[cardGlobalTaskIDProperty] == globalTaskID {
		return false
	}
	props[cardGlobalTaskIDProperty] = globalTaskID
	return true
}

func (a *App) GetCardsForBoard(boardID string, page int, perPage int) ([]*model.Card, error) {
	if err := a.ensureCardTaskIDs(boardID); err != nil {
		return nil, err
	}
	if err := a.ensureCardGlobalTaskIDsForBoard(boardID); err != nil {
		return nil, err
	}

	opts := model.QueryBlocksOptions{
		BoardID:   boardID,
		BlockType: model.TypeCard,
		Page:      page,
		PerPage:   perPage,
	}

	blocks, err := a.store.GetBlocks(opts)
	if err != nil {
		return nil, err
	}

	cards := make([]*model.Card, 0, len(blocks))
	for _, blk := range blocks {
		b := blk
		if card, err := model.Block2Card(b); err != nil {
			return nil, fmt.Errorf("Block2Card fail: %w", err)
		} else {
			cards = append(cards, card)
		}
	}
	return cards, nil
}

func (a *App) PatchCard(cardPatch *model.CardPatch, cardID string, userID string, disableNotify bool) (*model.Card, error) {
	blockPatch, err := model.CardPatch2BlockPatch(cardPatch)
	if err != nil {
		return nil, err
	}

	newBlock, err := a.PatchBlockAndNotify(cardID, blockPatch, userID, disableNotify)
	if err != nil {
		return nil, fmt.Errorf("cannot patch card %s: %w", cardID, err)
	}

	newCard, err := model.Block2Card(newBlock)
	if err != nil {
		return nil, err
	}

	return newCard, nil
}

func (a *App) GetCardByID(cardID string) (*model.Card, error) {
	cardBlock, err := a.GetBlockByID(cardID)
	if err != nil {
		return nil, err
	}

	card, err := model.Block2Card(cardBlock)
	if err != nil {
		return nil, err
	}

	return card, nil
}
