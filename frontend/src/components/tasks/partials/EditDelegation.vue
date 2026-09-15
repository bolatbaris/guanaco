<template>
	<div class="edit-delegation">
		<Multiselect
			v-model="selectedDelegation"
			class="edit-delegation-select"
			:loading="loading"
			:placeholder="modelValue ? $t('task.delegation.changePlaceholder') : $t('task.delegation.placeholder')"
			:search-results="delegationNames"
			label="name"
			:creatable="true"
			:no-results-message="noResultsMessage"
			:normalize-exact-match="true"
			:create-placeholder="$t('task.delegation.createPlaceholder')"
			:select-placeholder="$t('task.delegation.selectPlaceholder')"
			:show-empty="true"
			:search-delay="200"
			:disabled="disabled || isMutating"
			:aria-label="$t('task.delegation.searchLabel')"
			@search="findDelegationNames"
			@select="delegateSelectedName"
			@create="delegateTypedName"
			@focusin="preloadDelegationNames"
			@update:modelValue="handleSelectionUpdate"
		>
			<template #searchResult="{option}">
				<span class="delegation-option">
					<span
						class="delegation-option-avatar"
						aria-hidden="true"
					>
						<Icon icon="user" />
					</span>
					<span class="delegation-option-name">
						{{ typeof option === 'string' ? option : option.name }}
					</span>
				</span>
			</template>
		</Multiselect>

		<div
			v-if="modelValue"
			class="edit-delegation-actions"
		>
			<span class="edit-delegation-help">
				{{ $t('task.delegation.currentlyDelegated', {name: modelValue}) }}
			</span>
			<BaseButton
				class="take-back-button"
				:disabled="disabled || isMutating"
				:aria-label="$t('task.delegation.takeBackAriaLabel')"
				@click="clearDelegation"
			>
				<Icon icon="undo" />
				{{ $t('task.delegation.takeBack') }}
			</BaseButton>
		</div>
	</div>
</template>

<script setup lang="ts">
import {computed, ref, watch} from 'vue'
import {useI18n} from 'vue-i18n'

import BaseButton from '@/components/base/BaseButton.vue'
import Icon from '@/components/misc/Icon'
import Multiselect from '@/components/input/Multiselect.vue'

import {error, success} from '@/message'
import type {IDelegationName} from '@/modelTypes/IDelegationName'
import {useTaskDelegationService} from '@/services/taskDelegation'

const props = withDefaults(defineProps<{
	modelValue: string | undefined
	taskId: number
	disabled?: boolean
}>(), {
	disabled: false,
})

const emit = defineEmits<{
	'update:modelValue': [value: string | undefined]
}>()

const {t} = useI18n({useScope: 'global'})
const taskDelegationService = useTaskDelegationService()

const delegationNames = ref<IDelegationName[]>([])
const selectedDelegation = ref<IDelegationName | null>(null)
const isLoadingNames = ref(false)
const isMutating = ref(false)
const noResultsMessage = computed(() => isLoadingNames.value ? '' : t('task.delegation.noResults'))
const loading = computed(() => isLoadingNames.value || isMutating.value)

let hasPreloaded = false
let searchRequestId = 0

function normalizeName(name: string): string {
	return name.trim().replace(/\s+/g, ' ')
}

function namesMatch(left: string, right: string): boolean {
	return normalizeName(left).toLowerCase() === normalizeName(right).toLowerCase()
}

function optionForName(name: string): IDelegationName {
	return delegationNames.value.find(option => namesMatch(option.name, name)) ?? {
		id: 0,
		name,
		usageCount: 0,
		maxPermission: null,
	}
}

function syncSelectedDelegation(name: string | undefined) {
	const normalizedName = normalizeName(name ?? '')
	selectedDelegation.value = normalizedName === '' ? null : optionForName(normalizedName)
}

watch(
	() => props.modelValue,
	(value) => syncSelectedDelegation(value),
	{immediate: true},
)

watch(
	() => props.taskId,
	() => {
		hasPreloaded = false
		delegationNames.value = []
		searchRequestId++
	},
)

async function findDelegationNames(query = '') {
	const requestId = ++searchRequestId
	isLoadingNames.value = true

	try {
		const result = await taskDelegationService.getAll({
			q: query,
			page: 1,
			perPage: 50,
		})

		if (requestId === searchRequestId) {
			delegationNames.value = result.items
		}
	} catch (e) {
		if (requestId === searchRequestId) {
			delegationNames.value = []
			error(e)
		}
	} finally {
		if (requestId === searchRequestId) {
			isLoadingNames.value = false
		}
	}
}

function preloadDelegationNames() {
	if (props.disabled || isMutating.value || hasPreloaded) {
		return
	}

	hasPreloaded = true
	void findDelegationNames()
}

async function delegateName(name: string) {
	const normalizedName = normalizeName(name)
	if (normalizedName === '' || props.disabled || isMutating.value) {
		return
	}

	isMutating.value = true

	try {
		const updatedTask = await taskDelegationService.delegate(props.taskId, normalizedName)
		const delegatedName = normalizeName(updatedTask?.delegatedTo || normalizedName)

		emit('update:modelValue', delegatedName)
		selectedDelegation.value = optionForName(delegatedName)
		success({message: t('task.delegation.delegateSuccess', {name: delegatedName})})
	} catch (e) {
		syncSelectedDelegation(props.modelValue)
		error(e)
	} finally {
		isMutating.value = false
	}
}

function delegateSelectedName(name: IDelegationName) {
	void delegateName(name.name)
}

function delegateTypedName(name: string) {
	void delegateName(name)
}

function handleSelectionUpdate(value: IDelegationName | null) {
	if (value === null && props.modelValue) {
		void clearDelegation()
	}
}

async function clearDelegation() {
	if (!props.modelValue || props.disabled || isMutating.value) {
		return
	}

	isMutating.value = true

	try {
		await taskDelegationService.clear(props.taskId)
		selectedDelegation.value = null
		emit('update:modelValue', '')
		success({message: t('task.delegation.clearSuccess')})
	} catch (e) {
		syncSelectedDelegation(props.modelValue)
		error(e)
	} finally {
		isMutating.value = false
	}
}
</script>

<style lang="scss" scoped>
.edit-delegation {
	inline-size: 100%;
}

.edit-delegation-actions {
	display: flex;
	align-items: center;
	justify-content: space-between;
	gap: .75rem;
	flex-wrap: wrap;
	margin-block-start: .5rem;
}

.edit-delegation-help {
	min-inline-size: 0;
	color: var(--grey-600);
	font-size: .875rem;
	overflow-wrap: anywhere;
}

.take-back-button {
	display: inline-flex;
	align-items: center;
	gap: .35rem;
	padding: .35rem .6rem;
	border: 1px solid var(--grey-300);
	border-radius: $radius;
	background: var(--white);
	color: var(--grey-700);
	white-space: nowrap;

	&:hover:not(:disabled),
	&:focus-visible {
		border-color: var(--grey-500);
		background: var(--grey-100);
	}
}

.delegation-option {
	display: inline-flex;
	align-items: center;
	gap: .5rem;
	min-inline-size: 0;
	max-inline-size: 100%;
}

.delegation-option-avatar {
	display: inline-flex;
	flex: 0 0 auto;
	align-items: center;
	justify-content: center;
	inline-size: 1.5rem;
	block-size: 1.5rem;
	border-radius: 50%;
	background: var(--grey-200);
	color: var(--grey-600);
	font-size: .75rem;
}

.delegation-option-name {
	min-inline-size: 0;
	overflow: hidden;
	text-overflow: ellipsis;
	white-space: nowrap;
}
</style>
