import {createApp} from 'vue'
import {VueQueryPlugin} from '@tanstack/vue-query'

import pinia from './pinia'
import router from './router'
import App from './App.vue'
import {error, success} from './message'
import {configureApiClient} from './client/http'
import {queryClient} from './client/queryClient'
import {normalizeApiUrl} from './helpers/fetcher'

// Notifications
import Notifications from '@kyvg/vue3-notification'

// PWA
import './registerServiceWorker'

// i18n
import {getBrowserLanguage, i18n, setLanguage} from './i18n'

declare global {
	interface Window {
		API_URL: string;
		SENTRY_ENABLED?: boolean;
		SENTRY_DSN?: string;
		SENTRY_ENVIRONMENT?: string;
		SENTRY_FRONTEND_TRACES_SAMPLE_RATE?: number | string;
		SENTRY_FRONTEND_REPLAY_SESSION_SAMPLE_RATE?: number | string;
		SENTRY_FRONTEND_REPLAY_ON_ERROR_SAMPLE_RATE?: number | string;
		CUSTOM_LOGO_URL?: string;
		CUSTOM_LOGO_URL_DARK?: string;
	}
}

// Check if we have an api url in local storage and use it if that's the case
const apiUrlFromStorage = localStorage.getItem('API_URL')
if (apiUrlFromStorage !== null) {
	window.API_URL = apiUrlFromStorage
}

// Migrate persisted server URLs from the removed v1 API before any request is created.
window.API_URL = normalizeApiUrl(window.API_URL)

// Make sure the api url does not contain a / at the end
if (window.API_URL.endsWith('/')) {
	window.API_URL = window.API_URL.slice(0, -1)
}

if (apiUrlFromStorage !== null && apiUrlFromStorage !== window.API_URL) {
	localStorage.setItem('API_URL', window.API_URL)
}

configureApiClient()

// directives
import focus from '@/directives/focus'
import tooltip from '@/directives/tooltip'
import 'floating-vue/dist/style.css'
import shortcut from '@/directives/shortcut'

// global components
import FontAwesomeIcon from '@/components/misc/Icon'
import Button from '@/components/input/Button.vue'
import Modal from '@/components/misc/Modal.vue'
import Card from '@/components/misc/Card.vue'

import {setupKeyboardModality} from '@/helpers/keyboardModality'
import {handleChunkLoadErrors} from '@/helpers/handleChunkLoadErrors'

setupKeyboardModality()
handleChunkLoadErrors()

// We're loading the language before creating the app so that it won't fail to load when the user's 
// language file is not yet loaded.
const browserLanguage = getBrowserLanguage()
setLanguage(browserLanguage).then(async () => {
	const app = createApp(App)

	app.config.errorHandler = (err, vm, info) => {
		if (import.meta.env.DEV) {
			console.error(err, vm, info)
		}
		error(err)
	}

	if (import.meta.env.DEV) {
		app.config.warnHandler = (msg) => {
			error(msg)
			throw msg
		}

		// https://stackoverflow.com/a/52076738/15522256
		window.addEventListener('error', (err) => {
			error(err)
			throw err
		})


		window.addEventListener('unhandledrejection', (err) => {
			// event.promise contains the promise object
			// event.reason contains the reason for the rejection
			error(err)
			throw err
		})
	}

	const sentryEnabled = typeof window.SENTRY_ENABLED === 'boolean'
		? window.SENTRY_ENABLED
		: typeof import.meta.env.VITE_SENTRY_ENABLED === 'string'
			? import.meta.env.VITE_SENTRY_ENABLED === 'true'
			: Boolean(import.meta.env.VITE_SENTRY_DSN?.trim())

	if (sentryEnabled) {
		try {
			const sentry = await import('./sentry')
			await sentry.default(app, router)
		} catch (e) {
			console.error('Could not enable Sentry tracking', e)
		}
	}

	app.use(Notifications)
	app.use(VueQueryPlugin, {queryClient})

	app.directive('focus', focus)
	app.directive('tooltip', tooltip)
	app.directive('shortcut', shortcut)

	app.component('Icon', FontAwesomeIcon)
	app.component('XButton', Button)
	app.component('Modal', Modal)
	app.component('Card', Card)

	app.config.globalProperties.$message = {
		error,
		success,
	}

	app.use(pinia)
	app.use(router)
	app.use(i18n)

	app.mount('#app')
})
