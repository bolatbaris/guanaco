import {AuthenticatedHTTPFactory, apiV2Url} from '@/helpers/fetcher'
import {objectToCamelCase} from '@/helpers/case'
import {invalidateCachedTask} from '@/helpers/taskCache'

import type {IDelegationName} from '@/modelTypes/IDelegationName'
import type {ITask} from '@/modelTypes/ITask'

export interface DelegationNameListParams {
	q?: string
	page?: number
	perPage?: number
}

export interface DelegationNameListResult {
	items: IDelegationName[]
	total: number
	page: number
	perPage: number
	totalPages: number
}

interface DelegationNameListResponse {
	items?: Record<string, unknown>[]
	total?: number
	page?: number
	per_page?: number
	total_pages?: number
}

function parseDelegationName(raw: Record<string, unknown>): IDelegationName {
	const parsed = objectToCamelCase(raw)
	return {
		...parsed,
		maxPermission: parsed.maxPermission ?? null,
	} as IDelegationName
}

function parseTask(raw: Record<string, unknown>): ITask {
	return objectToCamelCase(raw) as ITask
}

export function useTaskDelegationService() {
	const http = AuthenticatedHTTPFactory()

	async function getAll(params: DelegationNameListParams = {}): Promise<DelegationNameListResult> {
		const {data} = await http.get<DelegationNameListResponse>(apiV2Url('delegation-names'), {
			params: {
				q: params.q,
				page: params.page,
				per_page: params.perPage,
			},
		})

		const total = data.total ?? 0
		const perPage = data.per_page ?? params.perPage ?? 0

		return {
			items: (data.items ?? []).map(parseDelegationName),
			total,
			page: data.page ?? params.page ?? 1,
			perPage,
			totalPages: data.total_pages ?? (perPage > 0 ? Math.ceil(total / perPage) : 0),
		}
	}

	async function delegate(taskId: ITask['id'], delegateeName: string): Promise<ITask> {
		const {data} = await http.post<Record<string, unknown>>(
			apiV2Url(`tasks/${taskId}/delegation`),
			{delegatee_name: delegateeName},
		)
		const task = parseTask(data)
		invalidateCachedTask(taskId)
		return task
	}

	async function clear(taskId: ITask['id']): Promise<void> {
		await http.delete(apiV2Url(`tasks/${taskId}/delegation`))
		invalidateCachedTask(taskId)
	}

	return {getAll, delegate, clear}
}
