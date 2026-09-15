import type {IRepeatAfter} from '@/types/IRepeatAfter'

export interface repeatParsedResult {
	textWithoutMatched: string,
	repeats: IRepeatAfter | null,
}

export interface ParsedTaskText {
	text: string,
	date: Date | null,
	labels: string[],
	project: string | null,
	repeats: IRepeatAfter | null,
}

export interface Prefixes {
	label: string,
	project: string,
}
