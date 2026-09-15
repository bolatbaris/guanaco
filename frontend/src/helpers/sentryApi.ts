import type {ResolvedRequestOptions} from '@/client/generated/client/types.gen'
import type {AxiosError} from 'axios'

type ApiErrorContext = {
	method?: string
	pathname?: string
	status?: number
}

const EXPECTED_CLIENT_ERROR_STATUSES = new Set([400, 401, 403, 409, 412, 413, 422])

let sentryModulePromise: Promise<typeof import('@sentry/vue')> | undefined

function getSentryModule(): Promise<typeof import('@sentry/vue')> {
	return sentryModulePromise ??= import('@sentry/vue')
}

export async function captureFrontendTestError(): Promise<boolean> {
	if (typeof window === 'undefined') {
		return false
	}

	const dsn = window.SENTRY_DSN?.trim() || import.meta.env.VITE_SENTRY_DSN?.trim()
	if (!dsn) {
		return false
	}

	try {
		const Sentry = await getSentryModule()
		if (!Sentry.getClient()) {
			return false
		}

		const eventId = Sentry.captureException(new Error('GlitchTip frontend test exception'), {
			tags: {
				glitchtip_test: 'true',
				component: 'frontend',
			},
		})
		return Boolean(eventId)
	} catch {
		return false
	}
}

function getPathname(request?: Request): string | undefined {
	if (!request) {
		return undefined
	}

	try {
		return new URL(request.url).pathname
	} catch {
		return undefined
	}
}

function getStatus(error: unknown, response?: Response): number | undefined {
	if (response) {
		return response.status
	}

	if (typeof error !== 'object' || error === null) {
		return undefined
	}

	const status = (error as {status?: unknown}).status
	return typeof status === 'number' ? status : undefined
}

function getAxiosStatus(error: AxiosError): number | undefined {
	const status = error.response?.status
	return typeof status === 'number' ? status : undefined
}

function getAxiosPathname(error: AxiosError): string | undefined {
	const url = error.config?.url
	if (!url) {
		return undefined
	}

	try {
		return new URL(url, error.config?.baseURL ?? window.location.origin).pathname
	} catch {
		return undefined
	}
}

function shouldCapture(status: number | undefined): boolean {
	return status === undefined || !EXPECTED_CLIENT_ERROR_STATUSES.has(status)
}

function getCategory(status: number | undefined): string {
	if (status === undefined) {
		return 'api_network_error'
	}

	if (status === 404) {
		return 'api_route_mismatch'
	}

	if (status === 429) {
		return 'api_rate_limited'
	}

	if (status >= 500) {
		return 'api_server_error'
	}

	return 'api_unexpected_client_error'
}

function createSafeError(category: string): Error {
	const error = new Error(category)
	error.name = 'ApiRequestError'
	return error
}

async function captureClassifiedError(
	status: number | undefined,
	context: ApiErrorContext,
): Promise<void> {
	if (typeof window === 'undefined' || !window.SENTRY_DSN?.trim() || !shouldCapture(status)) {
		return
	}

	try {
		const Sentry = await getSentryModule()
		if (!Sentry.getClient()) {
			return
		}

		const category = getCategory(status)
		Sentry.captureException(createSafeError(category), {
			tags: {
				api_error_category: category,
				...(context.method ? {method: context.method} : {}),
				...(context.status !== undefined ? {status: String(context.status)} : {}),
			},
			contexts: {
				api: context,
			},
		})
	} catch {
		// Observability must never change the generated client's error behavior.
	}
}

export async function captureApiError(
	error: unknown,
	response?: Response,
	request?: Request,
	options?: ResolvedRequestOptions,
): Promise<void> {
	const status = getStatus(error, response)
	await captureClassifiedError(status, {
		method: request?.method ?? options?.method,
		pathname: getPathname(request),
		status,
	})
}

export async function captureAxiosApiError(error: unknown): Promise<void> {
	if (!isAxiosError(error)) {
		return
	}

	const status = getAxiosStatus(error)
	await captureClassifiedError(status, {
		method: error.config?.method?.toUpperCase(),
		pathname: getAxiosPathname(error),
		status,
	})
}

function isAxiosError(error: unknown): error is AxiosError {
	return typeof error === 'object' && error !== null && (error as AxiosError).isAxiosError === true
}
