import {computed, defineAsyncComponent, h, shallowRef, type VNode, watchEffect} from 'vue'
import {useRoute, useRouter, type RouteLocationNormalizedGeneric} from 'vue-router'
import {useBaseStore} from '@/stores/base'

export function useRouteWithModal() {
	const router = useRouter()
	const route = useRoute()
	const baseStore = useBaseStore()
	const backdropView = computed(() => route.fullPath ? window.history.state?.backdropView : undefined)

	const routeWithModal = computed(() => {
		return backdropView.value
			? router.resolve(backdropView.value) as RouteLocationNormalizedGeneric
			: route
	})

	const currentModal = shallowRef<VNode>()
	watchEffect(() => {
		if (!backdropView.value) {
			currentModal.value = undefined
			return
		}

		const routePropsOption = route.matched[0]?.props.default
		let routeProps = undefined
		if (routePropsOption) {
			if (routePropsOption === true) {
				routeProps = route.params
			} else {
				if (typeof routePropsOption === 'function') {
					routeProps = routePropsOption(route)
				} else {
					routeProps = routePropsOption
				}
			}
		}

		if (typeof routeProps === 'undefined') {
			currentModal.value = undefined
			return
		}

		routeProps.backdropView = backdropView.value

		let component = route.matched[0]?.components?.default

		if (typeof component === 'function') {
			component = defineAsyncComponent(component)
		}

		if (!component) {
			currentModal.value = undefined
			return
		}
		currentModal.value = h(component, routeProps)
	})

	const historyState = computed(() => route.fullPath ? window.history.state : undefined)

	function closeModal() {
		// Try browser history first
		if (historyState.value?.back) {
			router.back()
			return
		}

		// Try backdrop view
		const backdropRoute = historyState.value?.backdropView && router.resolve(historyState.value.backdropView)
		if (backdropRoute && backdropRoute.params?.projectId !== '0') {
			router.push(backdropRoute)
			return
		}

		// Fallback to current project or home
		if (baseStore.currentProject && baseStore.currentProject.id !== 0) {
			router.push({
				name: 'project.index',
				params: { projectId: baseStore.currentProject.id },
			})
		} else {
			router.push({ name: 'home' })
		}
	}

	return {routeWithModal, currentModal, closeModal}
}
