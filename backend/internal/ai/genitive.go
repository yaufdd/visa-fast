package ai

import "strings"

// GenitiveFullName returns the surname+firstName in Russian genitive case
// (родительный падеж — отвечает на вопрос "кого?"), used for doverenost
// of a minor tourist. The second return is true if the algorithm was
// unable to handle the ending and the caller should add the suffix
// "[ПРОВЕРЬТЕ ПАДЕЖ]" — already appended in the first return.
// Rules cover the vast majority of Russian names; exotic endings fall
// back to the tagged form for manager review.
func GenitiveFullName(surname, firstName, gender string) (string, bool) {
	gSurname, okSurname := genitiveWord(surname, gender, true)
	gFirst, okFirst := genitiveWord(firstName, gender, false)
	full := strings.TrimSpace(gSurname + " " + gFirst)
	if !okSurname || !okFirst {
		return full + " [ПРОВЕРЬТЕ ПАДЕЖ]", true
	}
	return full, false
}

// genitiveWord returns genitive form of one word + a success flag.
func genitiveWord(word, gender string, isSurname bool) (string, bool) {
	if word == "" {
		return "", true
	}
	runes := []rune(word)
	last := runes[len(runes)-1]
	switch gender {
	case "Male":
		// ending "й" → replace with "я" (check before generic consonant rule)
		if last == 'й' {
			return string(runes[:len(runes)-1]) + "я", true
		}
		// consonant ending → add "а"
		if isRussianConsonant(last) {
			return word + "а", true
		}
	case "Female":
		if isSurname {
			// -ая → -ой
			if len(runes) >= 2 && runes[len(runes)-2] == 'а' && last == 'я' {
				return string(runes[:len(runes)-2]) + "ой", true
			}
			// -а → -ой
			if last == 'а' {
				return string(runes[:len(runes)-1]) + "ой", true
			}
		} else {
			// first name ending "я" → "и"
			if last == 'я' {
				return string(runes[:len(runes)-1]) + "и", true
			}
			// first name ending "а" after ж/ш/щ/ч → "и"; otherwise → "ы"
			if last == 'а' {
				if len(runes) >= 2 && isSoftConsonant(runes[len(runes)-2]) {
					return string(runes[:len(runes)-1]) + "и", true
				}
				return string(runes[:len(runes)-1]) + "ы", true
			}
		}
	}
	return word, false
}

func isRussianConsonant(r rune) bool {
	const consonants = "бвгджзйклмнпрстфхцчшщБВГДЖЗЙКЛМНПРСТФХЦЧШЩ"
	return strings.ContainsRune(consonants, r)
}

func isSoftConsonant(r rune) bool {
	return strings.ContainsRune("жшщчЖШЩЧ", r)
}
