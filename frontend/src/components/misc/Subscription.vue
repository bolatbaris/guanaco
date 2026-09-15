<template>
	<XButton
		v-if="type === 'button'"
		v-tooltip="tooltipText"
		variant="secondary"
		:icon="iconName"
		@click="changeSubscription"
	>
		{{ buttonText }}
	</XButton>
	<DropdownItem
		v-else-if="type === 'dropdown'"
		v-tooltip="tooltipText"
		:icon="iconName"
		@click="changeSubscription"
	>
		{{ buttonText }}
	</DropdownItem>
</template>

<script lang="ts" setup>
import {computed, shallowReactive} from 'vue'
import {useI18n} from 'vue-i18n'

import DropdownItem from '@/components/misc/DropdownItem.vue'

import SubscriptionService from '@/services/subscription'
import SubscriptionModel from '@/models/subscription'
import type {ISubscription} from '@/modelTypes/ISubscription'

import {success} from '@/message'
import type { IconProp } from '@fortawesome/fontawesome-svg-core'

const props = withDefaults(defineProps<{
	modelValue: ISubscription | null,
	entity: 'project',
	entityId: number,
	type?: 'button' | 'dropdown',
}>(), {
	type: 'button',
})

const emit = defineEmits<{
	'update:modelValue': [subscription: ISubscription | null]
}>()

const subscriptionService = shallowReactive(new SubscriptionService())

const {t} = useI18n({useScope: 'global'})

const isInherited = computed(() => props.modelValue !== null &&
	(props.modelValue.entity !== props.entity || props.modelValue.entityId !== props.entityId))

const tooltipText = computed(() => {
	if (isInherited.value) {
		return t('project.subscription.subscribedThroughParentProject')
	}

	return props.modelValue !== null ?
		t('project.subscription.subscribed') :
		t('project.subscription.notSubscribed')
})

const buttonText = computed(() => props.modelValue ? t('project.subscription.unsubscribe') : t('project.subscription.subscribe'))
const iconName = computed<IconProp>(() => props.modelValue ? ['far', 'bell-slash'] : 'bell')

function changeSubscription() {
	return props.modelValue === null
		? subscribe()
		: unsubscribe()
}

async function subscribe() {
	const subscription = new SubscriptionModel({
		entity: props.entity,
		entityId: props.entityId,
	})
	await subscriptionService.create(subscription)
	emit('update:modelValue', subscription)

	success({message: t('project.subscription.subscribeSuccess')})
}

async function unsubscribe() {
	const subscription = new SubscriptionModel({
		entity: props.entity,
		entityId: props.entityId,
	})
	await subscriptionService.delete(subscription)
	emit('update:modelValue', null)

	success({message: t('project.subscription.unsubscribeSuccess')})
}
</script>
