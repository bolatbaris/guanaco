import {computed, ref} from 'vue'
import {acceptHMRUpdate, defineStore} from 'pinia'
import router from '@/router'

import TaskService from '@/services/task'
import TaskDuplicateService from '@/services/taskDuplicateService'
import TaskDuplicateModel from '@/models/taskDuplicateModel'

import {parseTaskText} from '@/modules/quickAddMagic'

import TaskModel from '@/models/task'
import TaskReminderModel from '@/models/taskReminder'

import type {ITask} from '@/modelTypes/ITask'
import type {ITaskReminder} from '@/modelTypes/ITaskReminder'
import type {IAttachment} from '@/modelTypes/IAttachment'
import type {IProject} from '@/modelTypes/IProject'

import {REMINDER_PERIOD_RELATIVE_TO_TYPES} from '@/types/IReminderPeriodRelativeTo'

import {setModuleLoading} from '@/stores/helper'
import {useConfigStore} from '@/stores/config'
import {useProjectStore} from '@/stores/projects'
import {useKanbanStore} from '@/stores/kanban'
import {useBaseStore} from '@/stores/base'
import {useAuthStore} from '@/stores/auth'
import TaskCollectionService, {type TaskFilterParams} from '@/services/taskCollection'
import {getRandomColorHex} from '@/helpers/color/randomColor'
import {runWrites} from '@/helpers/runWrites'
import {toISOStringOrNull} from '@/helpers/time/toISOStringOrNull'
import {error} from '@/message'
import {REPEAT_TYPES} from '@/types/IRepeatAfter'
import {TASK_REPEAT_MODES} from '@/types/IRepeatMode'
import {taskLabelsCreate, taskLabelsDelete} from '@/client/generated'
import type {Label} from '@/client/generated'
import {
	createLabel,
	ensureLabels,
	getLabelByExactTitle,
	refreshLabels,
} from '@/client/queries/labels'

export function buildDefaultRemindersForQuickAdd(
	defaults: readonly ITaskReminder[] | undefined,
	dueDate: string | null,
): ITaskReminder[] {
	if (!dueDate) {
		return []
	}
	if (!defaults || defaults.length === 0) {
		return []
	}
	return defaults.map(d => new TaskReminderModel({
		reminder: null,
		relativePeriod: d.relativePeriod,
		relativeTo: REMINDER_PERIOD_RELATIVE_TO_TYPES.DUEDATE,
	}))
}

// Check if the label exists
function validateLabel(labels: Label[], label: string) {
	return getLabelByExactTitle(labels, label)
}

async function addLabelToTask(task: ITask, label: Label) {
	if (typeof label.id === 'undefined') {
		throw new Error('Cannot add a label without an id')
	}

	const {data} = await taskLabelsCreate({
		path: {projecttask: task.id},
		body: {label_id: label.id},
	})
	task.labels.push(label)
	return data
}

export const useTaskStore = defineStore('task', () => {
	const baseStore = useBaseStore()
	const kanbanStore = useKanbanStore()
	const projectStore = useProjectStore()
	const authStore = useAuthStore()
	const configStore = useConfigStore()

	const tasks = ref<{ [id: ITask['id']]: ITask }>({}) // TODO: or is this ITask[]
	const isLoading = ref(false)
	const lastUpdatedTask = ref<ITask | null>(null)

	const hasTasks = computed(() => Object.keys(tasks.value).length > 0)

	function setIsLoading(newIsLoading: boolean) {
		isLoading.value = newIsLoading
	}

	function setTasks(newTasks: ITask[]) {
		newTasks.forEach(task => {
			tasks.value[task.id] = task
		})
	}

	async function loadTasks(
		params: TaskFilterParams, 
		projectId: IProject['id'] | null = null,
	) {
		
		if (!params.filter_timezone || params.filter_timezone === '') {
			params.filter_timezone = authStore.settings.timezone
		}

		const cancel = setModuleLoading(setIsLoading)
		try {
			const model = {}
			let taskCollectionService = new TaskService()
			if (projectId !== null) {
				model.projectId = projectId
				taskCollectionService = new TaskCollectionService()
			}
			tasks.value = await taskCollectionService.getAll(model, params)
			baseStore.setHasTasks(tasks.value.length > 0)
			return tasks.value
		} finally {
			cancel()
		}
	}

	async function update(task: ITask) {
		const cancel = setModuleLoading(setIsLoading)

		const taskService = new TaskService()
		try {
			const updatedTask = await taskService.update(task)
			kanbanStore.ensureTaskIsInCorrectBucket(updatedTask)
			lastUpdatedTask.value = updatedTask
			return updatedTask
		} finally {
			cancel()
		}
	}

	async function deleteTask(task: ITask) {
		const taskService = new TaskService()
		const response = await taskService.delete(task)
		kanbanStore.removeTaskInBucket(task)
		return response
	}

	// Adds a task attachment in store.
	// This is an action to be able to commit other mutations
	function addTaskAttachment({
		taskId,
		attachment,
	}: {
		taskId: ITask['id']
		attachment: IAttachment
	}) {
		const t = kanbanStore.getTaskById(taskId)
		if (t.task !== null) {
			const attachments = [
				...t.task.attachments,
				attachment,
			]

			const newTask = {
				...t,
				task: {
					...t.task,
					attachments,
				},
			}
			kanbanStore.setTaskInBucketByIndex(newTask)
		}
	}

	async function addLabel({
		label,
		taskId,
	} : {
		label: Label,
		taskId: ITask['id']
	}) {
		if (typeof label.id === 'undefined') {
			throw new Error('Cannot add a label without an id')
		}

		const {data} = await taskLabelsCreate({
			path: {projecttask: taskId},
			body: {label_id: label.id},
		})
		const t = kanbanStore.getTaskById(taskId)
		if (t.task === null) {
			// Don't try further adding a label if the task is not in kanban
			// Usually this means the kanban board hasn't been accessed until now.
			// Vuex seems to have its difficulties with that, so we just log the error and fail silently.
			console.debug('Could not add label to task in kanban, task not found', {taskId, t})
			return data
		}

		kanbanStore.setTaskInBucketByIndex({
			...t,
			task: {
				...t.task,
				labels: [
					...t.task.labels,
					label,
				],
			},
		})

		return data
	}

	async function removeLabel(
		{label, taskId}:
		{label: Label, taskId: ITask['id']},
	) {
		if (typeof label.id === 'undefined') {
			throw new Error('Cannot remove a label without an id')
		}

		const {data} = await taskLabelsDelete({
			path: {projecttask: taskId, label: label.id},
		})
		const t = kanbanStore.getTaskById(taskId)
		if (t.task === null) {
			// Don't try further adding a label if the task is not in kanban
			// Usually this means the kanban board hasn't been accessed until now.
			// Vuex seems to have its difficulties with that, so we just log the error and fail silently.
			console.debug('Could not remove label from task in kanban, task not found', t)
			return data
		}

		// Remove the label from the project
		const labels = t.task.labels.filter(({ id }) => id !== label.id)

		kanbanStore.setTaskInBucketByIndex({
			...t,
			task: {
				...t.task,
				labels,
			},
		})

		return data
	}
	
	async function ensureLabelsExist(labels: string[]): Promise<Label[]> {
		const all = [...new Set(labels)]
		let availableLabels: Label[] = []
		let labelsLoaded = false
		try {
			availableLabels = await ensureLabels()
			labelsLoaded = true
		} catch (e) {
			console.debug('Could not load labels before creating them from quick add magic', e)
		}

		const hasMissingLabels = all.some(labelTitle => !validateLabel(availableLabels, labelTitle))
		if (labelsLoaded && hasMissingLabels) {
			try {
				availableLabels = await refreshLabels()
			} catch (e) {
				console.debug('Could not refresh labels before creating them from quick add magic', e)
			}
		}

		const mustCreateLabel = all.map(async labelTitle => {
			let label = validateLabel(availableLabels, labelTitle)
			if (typeof label === 'undefined') {
				try {
					label = await createLabel({
						title: labelTitle,
						hex_color: getRandomColorHex(),
					})
				} catch (e) {
					// Link shares may not create labels; skip it instead of aborting task creation.
					console.debug('Could not create label from quick add magic', {labelTitle, e})
					return undefined
				}
			}
			return label
		})
		const resolved = await Promise.all(mustCreateLabel)
		return resolved.filter((label): label is Label => typeof label !== 'undefined')
	}

	// Do everything that is involved in finding, creating and adding the label to the task
	async function addLabelsToTask(
		{ task, parsedLabels }:
		{ task: ITask, parsedLabels: string[] },
	) {
		if (parsedLabels.length <= 0) {
			return task
		}

		const labels = await ensureLabelsExist(parsedLabels)
		await runWrites(labels, l => addLabelToTask(task, l), configStore.concurrentWrites)
		return task
	}

	function findProjectId(
		{ project: projectName, projectId }:
		{ project: string, projectId: IProject['id'] }) {
		let foundProjectId = null

		// Uses the following ways to get the project id of the new task:
		//  1. If specified in quick add magic, look in store if it exists and use it if it does
		if (typeof projectName !== 'undefined' && projectName !== null) {
			let project = projectStore.findProjectByExactname(projectName)
			
			if (project === null) {
				project = projectStore.findProjectByIdentifier(projectName)
			}
			
			foundProjectId = project === null ? null : project.id
			if (foundProjectId !== null) {
				return foundProjectId
			}
		}
		
		//  2. Else check if a project was passed as parameter
		if (foundProjectId === null && projectId !== 0) {
			foundProjectId = projectId
		}
	
		//  3. Otherwise use the id from the route parameter
		const projectIdFromRoute = Number(router.currentRoute.value.params.projectId)
		if (typeof router.currentRoute.value.params.projectId !== 'undefined' && projectIdFromRoute > 0) {
			foundProjectId = projectIdFromRoute
		}
		
		//  4. If none of the above worked, reject the promise with an error.
		if (typeof foundProjectId === 'undefined' || projectId === null) {
			throw new Error('NO_PROJECT')
		}
	
		return foundProjectId
	}
	
	async function buildTaskFromQuickAddTitle({
		title,
		bucketId,
		projectId,
		position,
	} :
		Partial<ITask>,
	): Promise<{task: TaskModel, parsedLabels: string[]}> {
		const quickAddMagicMode = authStore.settings.frontendSettings.quickAddMagicMode
		const parsedTask = parseTaskText(title, quickAddMagicMode)

		if(parsedTask.text === '') {
			return {
				task: new TaskModel({
					title,
					projectId,
					bucketId,
					position,
				}),
				parsedLabels: [],
			}
		}

		const foundProjectId = await findProjectId({
			project: parsedTask.project,
			projectId: projectId || 0,
		})

		if(foundProjectId === null || foundProjectId === 0) {
			throw new Error('NO_PROJECT')
		}

		// I don't know why, but it all goes up in flames when I just pass in the date normally.
		const dueDate = toISOStringOrNull(parsedTask.date)

		const task = new TaskModel({
			title: parsedTask.text,
			projectId: foundProjectId,
			dueDate,
			bucketId: bucketId || 0,
			position,
		})
		task.repeatAfter = parsedTask.repeats
		task.reminders = buildDefaultRemindersForQuickAdd(
			authStore.settings.frontendSettings.quickAddDefaultReminders,
			dueDate,
		)

		if (parsedTask.repeats?.type === REPEAT_TYPES.Months && parsedTask.repeats?.amount === 1) {
			task.repeatMode = TASK_REPEAT_MODES.REPEAT_MODE_MONTH
		}

		return {task, parsedLabels: parsedTask.labels}
	}

	async function createNewTask({
		title,
		bucketId,
		projectId,
		position,
	} :
		Partial<ITask>,
	) {
		const cancel = setModuleLoading(setIsLoading)
		try {
			const {task, parsedLabels} = await buildTaskFromQuickAddTitle({
				title,
				bucketId,
				projectId,
				position,
			})

			const taskService = new TaskService()
			const createdTask = await taskService.create(task)
			return await addLabelsToTask({
				task: createdTask,
				parsedLabels,
			})
		} finally {
			cancel()
		}
	}

	// Returns the created tasks aligned 1:1 with entries (null = not created),
	// error is null when nothing failed.
	async function createNewTasksBulk(
		entries: {title: string, projectId: number}[],
	): Promise<{tasks: (ITask | null)[], error: unknown}> {
		const cancel = setModuleLoading(setIsLoading)
		try {
			const built = await Promise.all(entries.map(async ({title, projectId}) => {
				const {task, parsedLabels} = await buildTaskFromQuickAddTitle({title, projectId})
				return {task, parsedLabels}
			}))

			const taskService = new TaskService()
			const {tasks, error: bulkError} = await taskService.bulkCreate(built.map(b => b.task))

			const withLabels = built
				.map(({parsedLabels}, index) => ({task: tasks[index], parsedLabels}))
				.filter(c => c.task !== null && c.parsedLabels.length > 0)

			try {
				await runWrites(
					withLabels,
					c => addLabelsToTask({task: c.task as ITask, parsedLabels: c.parsedLabels}),
					configStore.concurrentWrites,
				)
			} catch (e) {
				// The tasks exist by now, so failing here must not look like the
				// whole creation failed — the caller would let the user resubmit.
				error(e)
			}

			return {tasks, error: bulkError}
		} finally {
			cancel()
		}
	}
	
	async function setCoverImage(task: ITask, attachment: IAttachment | null) {
		return update({
			...task,
			coverImageAttachmentId: attachment ? attachment.id : 0,
		})
	}
	
	async function duplicateTask(taskId: ITask['id']) {
		const cancel = setModuleLoading(setIsLoading)
		try {
			const taskDuplicateService = new TaskDuplicateService()
			const response = await taskDuplicateService.create(new TaskDuplicateModel({taskId}))
			return response.duplicatedTask
		} finally {
			cancel()
		}
	}

	async function markTaskAsRead(taskId: ITask['id']) {
		const taskService = new TaskService()
		await taskService.markTaskAsRead(taskId)
		
		const t = kanbanStore.getTaskById(taskId)
		if (t.task !== null) {
			kanbanStore.setTaskInBucket({
				...t.task,
				isUnread: false,
			})
		}
		
		if (tasks.value[taskId]) {
			tasks.value[taskId] = {
				...tasks.value[taskId],
				isUnread: false,
			}
		}
	}

	return {
		tasks,
		isLoading,
		lastUpdatedTask,

		hasTasks,

		setTasks,
		loadTasks,
		update,
		delete: deleteTask, // since delete is a reserved word we have to alias here
		addTaskAttachment,
		addLabel,
		removeLabel,
		addLabelsToTask,
		createNewTask,
		createNewTasksBulk,
		setCoverImage,
		findProjectId,
		ensureLabelsExist,
		duplicateTask,
		markTaskAsRead,
	}
})

// support hot reloading
if (import.meta.hot) {
	import.meta.hot.accept(acceptHMRUpdate(useTaskStore, import.meta.hot))
}
