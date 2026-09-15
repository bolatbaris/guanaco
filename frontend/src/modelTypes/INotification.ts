import type {IAbstract} from './IAbstract'
import type {IUser} from './IUser'
import type {ITask} from './ITask'
import type {ITaskComment} from './ITaskComment'
import type { IProject } from './IProject'

export const NOTIFICATION_NAMES = {
	'TASK_COMMENT': 'task.comment',
	'TASK_DELETED': 'task.deleted',
	'TASK_CREATED': 'task.created',
	'TASK_REMINDER': 'task.reminder',
	'PROJECT_CREATED': 'project.created',
	'TASK_MENTIONED': 'task.mentioned',
} as const

interface Notification {
	doer: IUser
}

interface NotificationTaskComment extends Notification {
	task: ITask
	comment: ITaskComment
}

interface NotificationTask extends Notification {
	task: ITask
}

interface NotificationCreated extends Notification {
	task: ITask
	project: IProject
}

interface NotificationTaskReminder extends Notification {
	task: ITask
	project: IProject
}

export interface INotification extends IAbstract {
	id: number
	name: string
	notification: NotificationTaskComment | NotificationTask | NotificationCreated | NotificationTaskReminder
	read: boolean
	readAt: Date | null

	created: Date
}
