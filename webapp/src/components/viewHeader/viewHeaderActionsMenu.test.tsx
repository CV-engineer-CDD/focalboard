// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import React from 'react'
import {render, screen} from '@testing-library/react'
import {Provider as ReduxProvider} from 'react-redux'

import '@testing-library/jest-dom'
import userEvent from '@testing-library/user-event'

import {mocked} from 'jest-mock'

import {TestBlockFactory} from '../../test/testBlockFactory'
import mutator from '../../mutator'

import {wrapIntl, mockStateStore} from '../../testUtils'

import {Archiver} from '../../archiver'

import {CsvExporter} from '../../csvExporter'

import ViewHeaderActionsMenu from './viewHeaderActionsMenu'

jest.mock('../../archiver')
jest.mock('../../csvExporter')
jest.mock('../../mutator')
const mockedArchiver = mocked(Archiver, true)
const mockedCsvExporter = mocked(CsvExporter, true)
const mockedMutator = mocked(mutator, true)

const board = TestBlockFactory.createBoard()
const activeView = TestBlockFactory.createBoardView(board)
const card = TestBlockFactory.createCard(board)
const deletedCard = TestBlockFactory.createCard(board)
deletedCard.id = 'deleted-card-id'
deletedCard.title = 'Deleted card'
deletedCard.deleteAt = 1680000000000
deletedCard.fields.taskId = '#42'
deletedCard.fields.globalTaskId = 'G-42'

describe('components/viewHeader/viewHeaderActionsMenu', () => {
    const state = {
        users: {
            me: {
                id: 'user-id-1',
                username: 'username_1',
            },
        },
        cards: {
            deletedCards: {
                [deletedCard.id]: deletedCard,
            },
        },
    }
    const store = mockStateStore([], state)
    beforeEach(() => {
        jest.clearAllMocks()
    })

    test('return menu', () => {
        const {container} = render(
            wrapIntl(
                <ReduxProvider store={store}>
                    <ViewHeaderActionsMenu
                        board={board}
                        activeView={activeView}
                        cards={[card]}
                    />
                </ReduxProvider>,
            ),
        )
        const buttonElement = screen.getByRole('button', {
            name: 'View header menu',
        })
        userEvent.click(buttonElement)
        expect(container).toMatchSnapshot()
    })

    test('return menu and verify call to csv exporter', () => {
        const {container} = render(
            wrapIntl(
                <ReduxProvider store={store}>
                    <ViewHeaderActionsMenu
                        board={board}
                        activeView={activeView}
                        cards={[card]}
                    />
                </ReduxProvider>,
            ),
        )
        const buttonElement = screen.getByRole('button', {name: 'View header menu'})
        userEvent.click(buttonElement)
        expect(container).toMatchSnapshot()
        const buttonExportCSV = screen.getByRole('button', {name: 'Export to CSV'})
        userEvent.click(buttonExportCSV)
        expect(mockedCsvExporter.exportTableCsv).toBeCalledTimes(1)
    })

    test('return menu and verify call to board archive', () => {
        const {container} = render(
            wrapIntl(
                <ReduxProvider store={store}>
                    <ViewHeaderActionsMenu
                        board={board}
                        activeView={activeView}
                        cards={[card]}
                    />
                </ReduxProvider>,
            ),
        )
        const buttonElement = screen.getByRole('button', {name: 'View header menu'})
        userEvent.click(buttonElement)
        expect(container).toMatchSnapshot()
        const buttonExportBoardArchive = screen.getByRole('button', {name: 'Export board archive'})
        userEvent.click(buttonExportBoardArchive)
        expect(mockedArchiver.exportBoardArchive).toBeCalledTimes(1)
        expect(mockedArchiver.exportBoardArchive).toBeCalledWith(board)
    })

    test('opens deleted cards dialog and restores a card', async () => {
        mockedMutator.undeleteBlock.mockResolvedValue(deletedCard)
        render(
            wrapIntl(
                <ReduxProvider store={store}>
                    <ViewHeaderActionsMenu
                        board={board}
                        activeView={activeView}
                        cards={[card]}
                    />
                </ReduxProvider>,
            ),
        )

        userEvent.click(screen.getByRole('button', {name: 'View header menu'}))
        userEvent.click(screen.getByRole('button', {name: 'Deleted cards (1)'}))
        expect(screen.getByRole('dialog')).toBeInTheDocument()
        expect(screen.getByText('Deleted card')).toBeInTheDocument()
        expect(screen.getByText('#42')).toBeInTheDocument()
        expect(screen.getByText('G-42')).toBeInTheDocument()

        userEvent.click(screen.getByRole('button', {name: 'Restore'}))

        expect(mockedMutator.undeleteBlock).toBeCalledWith(deletedCard, 'restore card')
    })

    test('opens deleted cards dialog and permanently deletes a card after confirmation', async () => {
        mockedMutator.permanentlyDeleteBlock.mockResolvedValue()
        render(
            wrapIntl(
                <ReduxProvider store={store}>
                    <ViewHeaderActionsMenu
                        board={board}
                        activeView={activeView}
                        cards={[card]}
                    />
                </ReduxProvider>,
            ),
        )

        userEvent.click(screen.getByRole('button', {name: 'View header menu'}))
        userEvent.click(screen.getByRole('button', {name: 'Deleted cards (1)'}))
        userEvent.click(screen.getAllByRole('button', {name: 'Permanently delete'})[0])
        expect(screen.getByTitle('Confirmation Dialog Box')).toBeInTheDocument()

        userEvent.click(screen.getAllByRole('button', {name: 'Permanently delete'})[1])

        expect(mockedMutator.permanentlyDeleteBlock).toBeCalledWith(deletedCard)
    })
})
