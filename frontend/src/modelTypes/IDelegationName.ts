import type {IAbstract} from './IAbstract'

export interface IDelegationName extends IAbstract {
	id: number
	name: string
	usageCount: number
}
