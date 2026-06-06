// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import React, {useState} from 'react'
import {FormattedMessage, useIntl} from 'react-intl'

import {Card} from '../../blocks/card'
import mutator from '../../mutator'
import Button from '../../widgets/buttons/button'
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
    const dialogTitle = (
        <FormattedMessage
            id='DeletedCardsDialog.title'
            defaultMessage='Deleted cards'
        />
    )

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

    return (
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
                            const taskID = card.fields.taskId || card.id
                            const globalTaskID = card.fields.globalTaskId || ''
                            const deletedAt = card.deleteAt ? Utils.displayDateTime(new Date(card.deleteAt), intl) : ''

                            return (
                                <div
                                    className='DeletedCardsDialog__row'
                                    key={card.id}
                                >
                                    <div className='DeletedCardsDialog__main'>
                                        <div className='DeletedCardsDialog__titleRow'>
                                            <span className='DeletedCardsDialog__taskID'>{taskID}</span>
                                            {globalTaskID && <span className='DeletedCardsDialog__globalID'>{globalTaskID}</span>}
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
                                    <Button
                                        size='small'
                                        filled={true}
                                        disabled={restoringCardID === card.id}
                                        onClick={() => restoreCard(card)}
                                    >
                                        <FormattedMessage
                                            id='DeletedCardsDialog.restore'
                                            defaultMessage='Restore'
                                        />
                                    </Button>
                                </div>
                            )
                        })}
                    </div>
                }
            </div>
        </Dialog>
    )
}

export default React.memo(DeletedCardsDialog)
