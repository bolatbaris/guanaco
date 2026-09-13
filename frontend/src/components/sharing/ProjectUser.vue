<template>
	<div>
		<h3 class="has-text-weight-bold share-heading">
			{{ $t('project.share.userTeam.shared', {type: shareTypeNames}) }}
		</h3>
		<div v-if="userIsAdmin">
			<div class="field has-addons">
				<p
					class="control is-expanded"
					:class="{ 'is-loading': searchService.loading }"
				>
					<Multiselect
						v-model="sharable"
						:loading="searchService.loading"
						:placeholder="$t('misc.searchPlaceholder')"
						:aria-label="$t('project.share.userTeam.search', {type: shareTypeName})"
						:search-results="found"
						label="username"
						@search="find"
					>
						<template #searchResult="{option: result}">
							<User
								:avatar-size="24"
								:show-username="true"
								:user="asUser(result)"
							/>
						</template>
					</Multiselect>
				</p>
				<p class="control">
					<XButton @click="add()">
						{{ $t('project.share.share') }}
					</XButton>
				</p>
			</div>
		</div>

		<div
			v-if="sharables.length > 0"
			class="has-horizontal-overflow mbe-4"
		>
			<table class="table has-actions is-striped is-hoverable is-fullwidth">
				<tbody>
					<tr
						v-for="s in sharables"
						:key="s.id"
					>
						<td>{{ getDisplayName(s) }}</td>
						<td>
							<template v-if="s.id === currentUserId">
								<b class="is-success">{{ $t('project.share.userTeam.you') }}</b>
							</template>
						</td>
						<td class="type">
							<template v-if="s.permission === PERMISSIONS.ADMIN">
								<span class="icon is-small">
									<Icon icon="lock" />
								</span>
								{{ $t('project.share.permission.admin') }}
							</template>
							<template v-else-if="s.permission === PERMISSIONS.READ_WRITE">
								<span class="icon is-small">
									<Icon icon="pen" />
								</span>
								{{ $t('project.share.permission.readWrite') }}
							</template>
							<template v-else>
								<span class="icon is-small">
									<Icon icon="users" />
								</span>
								{{ $t('project.share.permission.read') }}
							</template>
						</td>
						<td
							v-if="userIsAdmin"
							class="actions"
						>
							<div class="select">
								<select
									v-model="selectedPermission[s.id]"
									class="mie-2"
									:aria-label="$t('project.share.userTeam.permissionFor', {sharable: getDisplayName(s)})"
									@change="toggleType(s)"
								>
									<option :value="PERMISSIONS.READ">
										{{ $t('project.share.permission.read') }}
									</option>
									<option :value="PERMISSIONS.READ_WRITE">
										{{ $t('project.share.permission.readWrite') }}
									</option>
									<option :value="PERMISSIONS.ADMIN">
										{{ $t('project.share.permission.admin') }}
									</option>
								</select>
							</div>
							<XButton
								danger
								icon="trash-alt"
								:aria-label="$t('project.share.userTeam.remove', {type: shareTypeName})"
								@click="sharable = s; showDeleteModal = true"
							/>
						</td>
					</tr>
				</tbody>
			</table>
		</div>

		<Nothing v-else>
			{{ $t('project.share.userTeam.notShared', {type: shareTypeNames}) }}
		</Nothing>

		<Modal
			:enabled="showDeleteModal"
			@close="showDeleteModal = false"
			@submit="deleteSharable()"
		>
			<template #header>
				<span>{{
					$t('project.share.userTeam.removeHeader', {type: shareTypeName, sharable: sharableName})
				}}</span>
			</template>
			<template #text>
				<p>{{ $t('project.share.userTeam.removeText', {type: shareTypeName, sharable: sharableName}) }}</p>
			</template>
		</Modal>
	</div>
</template>

<script setup lang="ts">
import {computed, reactive, ref, shallowReactive, toRefs, toValue} from 'vue'
import {useI18n} from 'vue-i18n'

import UserProjectService from '@/services/userProject'
import UserProjectModel from '@/models/userProject'
import UserService from '@/services/user'
import UserModel, {getDisplayName} from '@/models/user'
import type {IUser} from '@/modelTypes/IUser'

import {PERMISSIONS, type Permission} from '@/constants/permissions'
import Multiselect from '@/components/input/Multiselect.vue'
import Nothing from '@/components/misc/Nothing.vue'
import Modal from '@/components/misc/Modal.vue'
import {success} from '@/message'
import {useAuthStore} from '@/stores/auth'
import User from '@/components/misc/User.vue'
import XButton from '@/components/input/Button.vue'

type SharedUser = IUser & {permission: Permission}

const props = withDefaults(defineProps<{
	id: number
	userIsAdmin?: boolean
}>(), {
	userIsAdmin: false,
})
const {id, userIsAdmin} = toRefs(props)

defineOptions({name: 'ProjectUserShare'})

const {t} = useI18n({useScope: 'global'})
const authStore = useAuthStore()
const searchService = shallowReactive(new UserService())
const stuffService = shallowReactive(new UserProjectService())
const stuffModel = reactive(new UserProjectModel({projectId: toValue(id)}))
const sharable = ref<IUser>(new UserModel())
const sharables = ref<SharedUser[]>([])
const found = ref<IUser[]>([])
const selectedPermission = ref<Record<number, Permission>>({})
const showDeleteModal = ref(false)

const shareTypeNames = computed(() => t('project.share.userTeam.typeUser', 2 as const))
const shareTypeName = computed(() => t('project.share.userTeam.typeUser', 1 as const))
const sharableName = computed(() => t('project.list.title'))
const currentUserId = computed(() => authStore.info?.id)

load()

async function load() {
	sharables.value = await stuffService.getAll(stuffModel) as unknown as SharedUser[]
	sharables.value.forEach(({id, permission}) => selectedPermission.value[id] = permission)
}

async function deleteSharable() {
	stuffModel.username = sharable.value.username
	await stuffService.delete(stuffModel)
	showDeleteModal.value = false
	const idx = sharables.value.findIndex(s => s.username === stuffModel.username)
	if (idx !== -1) {
		sharables.value.splice(idx, 1)
	}
	success({
		message: t('project.share.userTeam.removeSuccess', {
			type: shareTypeName.value,
			sharable: sharableName.value,
		}),
	})
}

async function add(admin = false) {
	const permission: Permission = admin ? PERMISSIONS.ADMIN : PERMISSIONS.READ
	stuffModel.permission = permission
	stuffModel.username = sharable.value.username
	await stuffService.create(stuffModel)
	success({message: t('project.share.userTeam.addedSuccess', {type: shareTypeName.value})})
	await load()
}

async function toggleType(sharedUser: SharedUser) {
	const permission = selectedPermission.value[sharedUser.id]
	if (![PERMISSIONS.ADMIN, PERMISSIONS.READ, PERMISSIONS.READ_WRITE].includes(permission)) {
		selectedPermission.value[sharedUser.id] = PERMISSIONS.READ
	}
	stuffModel.permission = selectedPermission.value[sharedUser.id]
	stuffModel.username = sharedUser.username
	const updated = await stuffService.update(stuffModel)
	sharedUser.permission = updated.permission
	success({message: t('project.share.userTeam.updatedSuccess', {type: shareTypeName.value})})
}

async function find(query: string) {
	if (query === '') {
		found.value = []
		return
	}

	const results = await searchService.getAll(new UserModel(), {s: query})
	found.value = results.filter(u =>
		u.id !== currentUserId.value && !sharables.value.some(s => s.id === u.id),
	)
}

function asUser(value: unknown): IUser {
	return value as IUser
}
</script>
