// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {Card} from './blocks/card'

export function displayCardGlobalID(card: Pick<Card, 'fields' | 'id'>): string {
    const globalTaskID = card.fields.globalTaskId || ''
    if (globalTaskID.startsWith('G-')) {
        return `#${globalTaskID.slice(2)}`
    }
    if (globalTaskID) {
        return globalTaskID
    }
    return card.fields.taskId || card.id
}

export function displayCardBoardID(card: Pick<Card, 'fields' | 'id'>): string {
    const taskID = card.fields.taskId || card.id
    return taskID.startsWith('#') ? taskID.slice(1) : taskID
}
