import type {ITask} from '@/modelTypes/ITask'
import type {IAttachment} from '@/modelTypes/IAttachment'
import type {IProject} from '@/modelTypes/IProject'
import type {IBucket} from '@/modelTypes/IBucket'

import type {IRepeatAfter} from '@/types/IRepeatAfter'
import type {IRelationKind} from '@/types/IRelationKind'
import {TASK_REPEAT_MODES, type IRepeatMode} from '@/types/IRepeatMode'
import type {Label} from '@/client/generated'

import {parseDateOrNull} from '@/helpers/parseDateOrNull'
import {secondsToPeriod} from '@/helpers/time/period'
import {objectToSnakeCase} from '@/helpers/case'

import AbstractModel from './abstractModel'
import UserModel from './user'
import AttachmentModel from './attachment'
import type {ITaskReminder} from '@/modelTypes/ITaskReminder'
import TaskReminderModel from '@/models/taskReminder'
import TaskCommentModel from '@/models/taskComment.ts'

/**
 * Parses `repeatAfterSeconds` into a usable js object.
 */
export function parseRepeatAfter(repeatAfterSeconds: number): IRepeatAfter {
	
	const period = secondsToPeriod(repeatAfterSeconds)
	
	return {
		type: period.unit,
		amount: period.amount,
	}
}

export function getTaskIdentifier(task: ITask | null | undefined): string {
	if (task === null || typeof task === 'undefined') {
		return ''
	}
	
	if (task.identifier === '') {
		return `#${task.index}`
	}

	return task.identifier
}

export default class TaskModel extends AbstractModel<ITask> implements ITask {
	id = 0
	title = ''
	description = ''
	done = false
	doneAt: Date | null = null
	deletedAt: Date | null = null
	labels: Label[] = []
	delegatedTo = ''

	dueDate: Date | null = 0
	startDate: Date | null = 0
	endDate: Date | null = 0
	repeatAfter: number | IRepeatAfter = 0
	repeatFromCurrentDate = false
	repeatMode: IRepeatMode = TASK_REPEAT_MODES.REPEAT_MODE_DEFAULT
	reminders: ITaskReminder[] = []
	parentTaskId: ITask['id'] = 0
	relatedTasks:  Partial<Record<IRelationKind, ITask[]>> = {}
	attachments: IAttachment[] = []
	coverImageAttachmentId: IAttachment['id'] = null
	identifier = ''
	index = 0
	position = 0
	
	reactions = {}
	comments = []

	createdBy: IUser = UserModel
	created: Date = null
	updated: Date = null

	projectId: IProject['id'] = 0
	bucketId: IBucket['id'] = 0
	buckets: IBucket[] = []

	constructor(data: Partial<ITask> = {}) {
		super()
		const taskData = {...data}

		const labels = (taskData.labels ?? []).map(label => objectToSnakeCase(label) as Label)
		this.assignData(taskData)

		this.id = Number(this.id)
		this.title = this.title?.trim()
		this.doneAt = parseDateOrNull(this.doneAt)
		this.deletedAt = parseDateOrNull(this.deletedAt)

		this.labels = labels.sort((a, b) => (a.title ?? '').localeCompare(b.title ?? ''))

		this.dueDate = parseDateOrNull(this.dueDate)
		this.startDate = parseDateOrNull(this.startDate)
		this.endDate = parseDateOrNull(this.endDate)

		// Parse the repeat after into something usable
		this.repeatAfter = parseRepeatAfter(this.repeatAfter as number)

		this.reminders = this.reminders.map(r => new TaskReminderModel(r))

		// Convert all subtasks to task models
		Object.keys(this.relatedTasks).forEach(relationKind => {
			this.relatedTasks[relationKind] = this.relatedTasks[relationKind].map(t => {
				return new TaskModel(t)
			})
		})

		// Make all attachments to attachment models
		this.attachments = this.attachments.map(a => new AttachmentModel(a))

		// Set the task identifier to empty if the project does not have one
		if (this.identifier === `-${this.index}`) {
			this.identifier = ''
		}

		this.createdBy = new UserModel(this.createdBy)
		this.created = new Date(this.created)
		this.updated = new Date(this.updated)

		this.projectId = Number(this.projectId)

		// If we would use the camel cased value here, it would lose the reactions - emojis can't be camel cased.
		// The comments will be camel cased anyway in the constructor of the task comment model.
		this.comments = (data.comments || []).map(c => new TaskCommentModel(c))

		// We can't convert emojis to camel case, hence we do this manually
		this.reactions = {}
		Object.keys(data.reactions || {}).forEach(reaction => {
			this.reactions[reaction] = data.reactions[reaction].map(u => new UserModel(u))
		})
	}

	getTextIdentifier() {
		return getTaskIdentifier(this)
	}

}
