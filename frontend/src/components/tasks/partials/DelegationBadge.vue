<template>
	<span
		class="delegation-badge"
		:class="{'is-compact': compact, 'is-inline': inline}"
		role="img"
		:aria-label="$t('task.delegation.badgeAriaLabel', {name})"
	>
		<span
			class="delegation-avatar"
			aria-hidden="true"
		>
			<Icon icon="user" />
		</span>
		<span class="delegation-content">
			<span class="delegation-label">{{ $t('task.delegation.badgeLabel') }}</span>
			<span
				class="delegation-name"
				:title="name"
			>
				{{ name }}
			</span>
		</span>
	</span>
</template>

<script setup lang="ts">
import Icon from '@/components/misc/Icon'

withDefaults(defineProps<{
	name: string
	compact?: boolean
	inline?: boolean
}>(), {
	compact: false,
	inline: false,
})
</script>

<style lang="scss" scoped>
.delegation-badge {
	display: inline-flex;
	align-items: center;
	gap: .5rem;
	max-inline-size: 100%;
	padding: .3rem .65rem .3rem .35rem;
	border: 1px solid var(--grey-300);
	border-radius: 999px;
	background: var(--grey-100);
	color: var(--grey-800);
	vertical-align: middle;
}

.delegation-avatar {
	display: inline-flex;
	flex: 0 0 auto;
	align-items: center;
	justify-content: center;
	inline-size: 1.75rem;
	block-size: 1.75rem;
	border-radius: 50%;
	background: var(--grey-200);
	color: var(--grey-600);
	font-size: .85rem;
}

.delegation-content {
	display: inline-flex;
	align-items: baseline;
	gap: .4rem;
	min-inline-size: 0;
}

.delegation-label {
	flex: 0 0 auto;
	color: var(--grey-600);
	font-size: .7rem;
	font-weight: 600;
	letter-spacing: .04em;
	line-height: 1.2;
	text-transform: uppercase;
}

.delegation-name {
	min-inline-size: 0;
	overflow: hidden;
	text-overflow: ellipsis;
	white-space: nowrap;
}

.delegation-badge.is-compact {
	gap: .35rem;
	padding: .2rem .45rem .2rem .25rem;

	.delegation-avatar {
		inline-size: 1.5rem;
		block-size: 1.5rem;
		font-size: .7rem;
	}

	.delegation-label {
		font-size: .65rem;
	}

	.delegation-name {
		font-size: .875rem;
	}
}

.delegation-badge.is-inline {
	vertical-align: middle;
}

@media screen and (max-width: $tablet) {
	.delegation-content {
		align-items: flex-start;
		flex-direction: column;
		gap: 0;
	}
}
</style>
