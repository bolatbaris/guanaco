import type {IAbstract} from './IAbstract'
import type {IAttachment} from './IAttachment'
import type {IProject} from './IProject'
import type {IBucket} from './IBucket'

import type {IRelationKind} from '@/types/IRelationKind'
import type {IRepeatAfter} from '@/types/IRepeatAfter'
import type {IRepeatMode} from '@/types/IRepeatMode'

import type {PartialWithId} from '@/types/PartialWithId'
import type {ITaskReminder} from '@/modelTypes/ITaskReminder'
import type {IReactionPerEntity} from '@/modelTypes/IReaction'
import type {ITaskComment} from '@/modelTypes/ITaskComment.ts'
import type {Label} from '@/client/generated'
import type {IUser} from '@/modelTypes/IUser'

export interface ITask extends IAbstract {
	id: number
	title: string
	description: string
	done: boolean
	doneAt: Date | null
	deletedAt: Date | null
	labels: Label[]
	delegatedTo?: string

	dueDate: Date | null
	startDate: Date | null
	endDate: Date | null
	repeatAfter: number | IRepeatAfter
	repeatFromCurrentDate: boolean
	repeatMode: IRepeatMode
	reminders: ITaskReminder[]
	parentTaskId: ITask['id']
	relatedTasks: Partial<Record<IRelationKind, ITask[]>>
	attachments: IAttachment[]
	coverImageAttachmentId: IAttachment['id'] | null
	identifier: string
	index: number
	isUnread?: boolean

	position: number
	
	reactions: IReactionPerEntity
	comments: ITaskComment[]
	commentCount?: number
	timeEntriesCount?: number

	createdBy: IUser
	created: Date
	updated: Date

	projectId: IProject['id'] // Used for task routing, reads, and creation
	bucketId: IBucket['id']
	buckets: IBucket[]
}

export type ITaskPartialWithId = PartialWithId<ITask>
