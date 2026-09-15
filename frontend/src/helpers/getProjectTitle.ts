import {i18n} from '@/i18n'
import type {IProject} from '@/modelTypes/IProject'

export function getProjectTitle(project: Pick<IProject, 'id' | 'title'>) {
	if (project.title === 'Inbox') {
		return i18n.global.t('project.inboxTitle')
	}

	if (project.title === 'My Open Tasks') {
		return i18n.global.t('project.myOpenTasksFilterTitle')
	}

	return project.title
}
