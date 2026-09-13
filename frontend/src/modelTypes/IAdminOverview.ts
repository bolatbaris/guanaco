import type {IAbstract} from './IAbstract'

export interface IAdminOverviewShares {
	linkShares: number
	userShares: number
}

export interface IAdminOverviewLicense {
	licensed: boolean
	instanceId: string
	features: string[]
	maxUsers: number
	expiresAt: Date
	validatedAt: Date
	lastCheckFailed: boolean
}

export interface IAdminOverview extends IAbstract {
	users: number
	projects: number
	tasks: number
	shares: IAdminOverviewShares
	license: IAdminOverviewLicense
}
