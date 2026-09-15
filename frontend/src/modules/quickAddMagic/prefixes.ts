import type {Prefixes} from './types'

const VIKUNJA_PREFIXES: Prefixes = {
	label: '*',
	project: '+',
}

const TODOIST_PREFIXES: Prefixes = {
	label: '@',
	project: '#',
}

export enum PrefixMode {
	Disabled = 'disabled',
	Default = 'vikunja',
	Todoist = 'todoist',
}

export const PREFIXES = {
	[PrefixMode.Disabled]: undefined,
	[PrefixMode.Default]: VIKUNJA_PREFIXES,
	[PrefixMode.Todoist]: TODOIST_PREFIXES,
}
