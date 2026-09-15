import type {IAbstract} from './IAbstract'
import type {IUser} from './IUser'

export interface ISubscription extends IAbstract {
	id: number
	entity: 'project'
	entityId: number
	user: IUser

	created: Date
}
