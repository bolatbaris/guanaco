<template>
	<div
		class="user"
		:class="{'is-inline': isInline}"
		:style="{'--avatar-size': `${avatarSize}px`}"
	>
		<span class="avatar-wrapper">
			<UserAvatar
				v-tooltip="displayName"
				:user="user"
				:size="avatarSize"
				:alt="showUsername ? '' : t('misc.avatarOfUser', {user: displayName})"
				class="avatar"
			/>
		</span>
		<span
			v-if="showUsername"
			class="username"
		>{{ displayName }}</span>
	</div>
</template>

<script lang="ts" setup>
import {computed} from 'vue'
import {useI18n} from 'vue-i18n'

import UserAvatar from '@/components/misc/UserAvatar.vue'
import {getDisplayName} from '@/models/user'
import type {IUser} from '@/modelTypes/IUser'

const props = withDefaults(defineProps<{
	user: IUser,
	showUsername?: boolean,
	avatarSize?: number,
	isInline?: boolean,
}>(), {
	showUsername: true,
	avatarSize: 50,
	isInline: false,
})

const {t} = useI18n({useScope: 'global'})

const displayName = computed(() => getDisplayName(props.user))
</script>

<style lang="scss" scoped>
.user {
	display: flex;
	justify-items: center;

	&.is-inline {
		display: inline-flex;
	}
}

.avatar-wrapper {
	position: relative;
	display: inline-flex;
	flex-shrink: 0;
	margin-inline-end: .5rem;
}

.avatar {
	inline-size: var(--avatar-size);
	block-size: var(--avatar-size);
	border-radius: 100%;
	vertical-align: middle;
}

</style>
