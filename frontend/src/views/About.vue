<template>
	<Modal
		variant="hint-modal"
		@close="$router.back()"
	>
		<Card
			class="has-no-shadow"
			:title="$t('about.title')"
			:padding="false"
			:show-close="true"
			@close="$router.back()"
		>
			<div class="p-4">
				<p v-if="versionsEqual">
					{{ $t('about.version', {version: apiVersion}) }}
				</p>
				<template v-else>
					<p>{{ $t('about.frontendVersion', {version: frontendVersion}) }}</p>
					<p>{{ $t('about.apiVersion', {version: apiVersion}) }}</p>
				</template>

				<div class="mbe-4">
					<p class="has-text-weight-bold">
						{{ $t('about.glitchtipTestTitle') }}
					</p>
					<p class="help">
						{{ $t('about.glitchtipTestDescription') }}
					</p>
					<div class="buttons">
						<XButton
							variant="secondary"
							:loading="frontendTestLoading"
							@click="sendFrontendTest"
						>
							{{ $t('about.testFrontendError') }}
						</XButton>
						<XButton
							variant="secondary"
							:loading="backendTestLoading"
							@click="sendBackendTest"
						>
							{{ $t('about.testBackendError') }}
						</XButton>
					</div>
					<p
						v-if="frontendTestState === 'sent'"
						class="help has-text-success"
					>
						{{ $t('about.frontendTestSent') }}
					</p>
					<p
						v-else-if="frontendTestState === 'unavailable'"
						class="help has-text-danger"
					>
						{{ $t('about.frontendTestUnavailable') }}
					</p>
					<p
						v-if="backendTestState === 'sent'"
						class="help has-text-success"
					>
						{{ $t('about.backendTestSent') }}
					</p>
					<p
						v-else-if="backendTestState === 'unavailable'"
						class="help has-text-danger"
					>
						{{ $t('about.backendTestUnavailable') }}
					</p>
				</div>
			</div>
			<template #footer>
				<XButton
					variant="secondary"
					@click.prevent.stop="$router.back()"
				>
					{{ $t('misc.close') }}
				</XButton>
			</template>
		</Card>
	</Modal>
</template>

<script setup lang="ts">
import {computed, ref} from 'vue'

import {VERSION as frontendVersion} from '@/version.json'

import {useConfigStore} from '@/stores/config'
import {captureFrontendTestError} from '@/helpers/sentryApi'
import {observabilityTestBackend} from '@/client/generated'

const configStore = useConfigStore()
const apiVersion = computed(() => configStore.version)
const versionsEqual = computed(() => apiVersion.value === frontendVersion)

const frontendTestLoading = ref(false)
const backendTestLoading = ref(false)
const frontendTestState = ref<'idle' | 'sent' | 'unavailable'>('idle')
const backendTestState = ref<'idle' | 'sent' | 'unavailable'>('idle')

async function sendFrontendTest() {
	frontendTestLoading.value = true
	frontendTestState.value = 'idle'
	frontendTestState.value = await captureFrontendTestError() ? 'sent' : 'unavailable'
	frontendTestLoading.value = false
}

async function sendBackendTest() {
	backendTestLoading.value = true
	backendTestState.value = 'idle'
	try {
		const {data} = await observabilityTestBackend()
		backendTestState.value = data.captured ? 'sent' : 'unavailable'
	} catch {
		backendTestState.value = 'unavailable'
	}
	backendTestLoading.value = false
}
</script>
