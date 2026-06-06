// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import React, {useState} from 'react'
import {FormattedMessage, useIntl} from 'react-intl'

import {Card} from '../../blocks/card'
import {displayCardBoardID, displayCardGlobalID} from '../../cardIDs'
import mutator from '../../mutator'
import Button from '../../widgets/buttons/button'
import ConfirmationDialogBox, {ConfirmationDialogBoxProps} from '../confirmationDialogBox'
import Dialog from '../dialog'
import {sendFlashMessage} from '../flashMessages'
import {Utils} from '../../utils'

import './deletedCardsDialog.scss'

type Props = {
    cards: Card[]
    onClose: () => void
}

const DeletedCardsDialog = (props: Props): JSX.Element => {
    const intl = useIntl()
    const [restoringCardID, setRestoringCardID] = useState('')
    const [deletingCardID, setDeletingCardID] = useState('')
    const [confirmDeleteCard, setConfirmDeleteCard] = useState<Card|null>(null)
    const dialogTitle = (
        <FormattedMessage
            id='DeletedCardsDialog.title'
            defaultMessage='Deleted cards'
        />
    )

    const deletedAtDate = (deleteAt: number): Date => {
        return new Date(deleteAt < 1000000000000 ? deleteAt * 1000 : deleteAt)
    }

    const restoreCard = async (card: Card) => {
        setRestoringCardID(card.id)
        try {
            await mutator.undeleteBlock(card, intl.formatMessage({id: 'DeletedCardsDialog.restore-action', defaultMessage: 'restore card'}))
            sendFlashMessage({
                content: intl.formatMessage({id: 'DeletedCardsDialog.restore-success', defaultMessage: 'Card restored.'}),
                severity: 'normal',
            })
        } catch (e) {
            Utils.logError(`RestoreCard ERROR: ${e}`)
            sendFlashMessage({
                content: intl.formatMessage({id: 'DeletedCardsDialog.restore-failed', defaultMessage: 'Unable to restore card.'}),
                severity: 'high',
            })
        } finally {
            setRestoringCardID('')
        }
    }

    const permanentlyDeleteCard = async (card: Card) => {
        setDeletingCardID(card.id)
        try {
            await mutator.permanentlyDeleteBlock(card)
            sendFlashMessage({
                content: intl.formatMessage({id: 'DeletedCardsDialog.delete-success', defaultMessage: 'Card permanently deleted.'}),
                severity: 'normal',
            })
        } catch (e) {
            Utils.logError(`PermanentlyDeleteCard ERROR: ${e}`)
            sendFlashMessage({
                content: intl.formatMessage({id: 'DeletedCardsDialog.delete-failed', defaultMessage: 'Unable to permanently delete card.'}),
                severity: 'high',
            })
        } finally {
            setDeletingCardID('')
            setConfirmDeleteCard(null)
        }
    }

    const confirmDeleteDialogProps: ConfirmationDialogBoxProps|null = confirmDeleteCard ? {
        heading: intl.formatMessage({id: 'DeletedCardsDialog.delete-confirm-heading', defaultMessage: 'Permanently delete card?'}),
        subText: intl.formatMessage({id: 'DeletedCardsDialog.delete-confirm-subtext', defaultMessage: 'This card will be removed from deleted cards and cannot be restored.'}),
        confirmButtonText: intl.formatMessage({id: 'DeletedCardsDialog.delete-confirm-button', defaultMessage: 'Permanently delete'}),
        destructive: true,
        onConfirm: () => permanentlyDeleteCard(confirmDeleteCard),
        onClose: () => setConfirmDeleteCard(null),
    } : null

    return (
        <>
            <Dialog
                className='DeletedCardsDialog'
                size='medium'
                title={dialogTitle}
                onClose={props.onClose}
            >
                <div className='DeletedCardsDialog__content'>
                    {props.cards.length === 0 &&
                        <div className='DeletedCardsDialog__empty'>
                            <FormattedMessage
                                id='DeletedCardsDialog.empty'
                                defaultMessage='No deleted cards.'
                            />
                        </div>
                    }
                    {props.cards.length > 0 &&
                        <div className='DeletedCardsDialog__list'>
                            {props.cards.map((card) => {
                                const title = card.title || intl.formatMessage({id: 'DeletedCardsDialog.untitled', defaultMessage: 'Untitled'})
                                const globalTaskID = displayCardGlobalID(card)
                                const boardTaskID = displayCardBoardID(card)
                                const deletedAt = card.deleteAt ? Utils.displayDateTime(deletedAtDate(card.deleteAt), intl) : ''

                                return (
                                    <div
                                        className='DeletedCardsDialog__row'
                                        key={card.id}
                                    >
                                        <div className='DeletedCardsDialog__main'>
                                            <div className='DeletedCardsDialog__titleRow'>
                                                <span className='DeletedCardsDialog__taskID'>{globalTaskID}</span>
                                                {boardTaskID &&
                                                    <span className='DeletedCardsDialog__globalID'>
                                                        <FormattedMessage
                                                            id='DeletedCardsDialog.board-id'
                                                            defaultMessage='Board {id}'
                                                            values={{id: boardTaskID}}
                                                        />
                                                    </span>
                                                }
                                                <span className='DeletedCardsDialog__title'>{title}</span>
                                            </div>
                                            {deletedAt &&
                                                <div className='DeletedCardsDialog__deletedAt'>
                                                    <FormattedMessage
                                                        id='DeletedCardsDialog.deleted-at'
                                                        defaultMessage='Deleted {time}'
                                                        values={{time: deletedAt}}
                                                    />
                                                </div>
                                            }
                                        </div>
                                        <div className='DeletedCardsDialog__actions'>
                                            <Button
                                                size='small'
                                                danger={true}
                                                disabled={deletingCardID === card.id || restoringCardID === card.id}
                                                onClick={() => setConfirmDeleteCard(card)}
                                            >
                                                <FormattedMessage
                                                    id='DeletedCardsDialog.permanently-delete'
                                                    defaultMessage='Permanently delete'
                                                />
                                            </Button>
                                            <Button
                                                size='small'
                                                filled={true}
                                                disabled={restoringCardID === card.id || deletingCardID === card.id}
                                                onClick={() => restoreCard(card)}
                                            >
                                                <FormattedMessage
                                                    id='DeletedCardsDialog.restore'
                                                    defaultMessage='Restore'
                                                />
                                            </Button>
                                        </div>
                                    </div>
                                )
                            })}
                        </div>
                    }
                </div>
            </Dialog>
            {confirmDeleteDialogProps && <ConfirmationDialogBox dialogBox={confirmDeleteDialogProps}/>}
        </>
    )
}

export default React.memo(DeletedCardsDialog)
