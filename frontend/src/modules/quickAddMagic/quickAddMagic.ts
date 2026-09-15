import {parseDate} from './dateParser'
import {PREFIXES, PrefixMode} from './prefixes'
import {getLabelsFromPrefix, getProjectFromPrefix} from './prefixParser'
import {getRepeats} from './repeatParser'
import {cleanupItemText, cleanupResult} from './textCleanup'
import type {ParsedTaskText} from './types'

/**
 * Parses task text for dates, labels, projects and returns an object with all found intents.
 *
 * @param text
 */
export const parseTaskText = (text: string, prefixesMode: PrefixMode = PrefixMode.Default, now: Date = new Date()): ParsedTaskText => {
	const result: ParsedTaskText = {
		text: text,
		date: null,
		labels: [],
		project: null,
		repeats: null,
	}

	// If the entire text is wrapped in quotes, strip them and skip all parsing
	if (
		text.length >= 2
		&& ((text.startsWith('"') && text.endsWith('"'))
			|| (text.startsWith('\'') && text.endsWith('\'')))
	) {
		result.text = text.slice(1, -1)
		return result
	}

	const prefixes = PREFIXES[prefixesMode]
	if (prefixes === undefined) {
		return result
	}

	result.labels = getLabelsFromPrefix(text, prefixesMode) ?? []
	result.text = cleanupItemText(result.text, result.labels, prefixes.label)

	result.project = getProjectFromPrefix(result.text, prefixesMode)
	result.text = result.project !== null ? cleanupItemText(result.text, [result.project], prefixes.project) : result.text

	const {textWithoutMatched, repeats} = getRepeats(result.text)
	result.text = textWithoutMatched
	result.repeats = repeats

	const {newText, date} = parseDate(result.text, now)
	result.text = newText
	result.date = date

	return cleanupResult(result, prefixes)
}
