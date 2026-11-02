// Suffixes below NO_SUFFIX are pre-releases that sort under the plain version (1.0_pre1 < 1.0 < 1.0_git1); keep in step with internal/version/version.go.
const SUFFIX_RANK: Record<string, number> = {
  alpha: 0, beta: 1, pre: 2, rc: 3,
  cvs: 5, svn: 6, git: 7, hg: 8, p: 9,
}

const NO_SUFFIX = 4

interface Parsed {
  numbers: number[]
  letter: number
  suffixes: { rank: number; number: number }[]
  revision: number
}

function leadingInt(s: string): number {
  const digits = s.match(/^\d+/)
  return digits ? parseInt(digits[0], 10) : 0
}

function parse(raw: string): Parsed {
  const parsed: Parsed = { numbers: [], letter: 0, suffixes: [], revision: 0 }
  let v = raw.replace(/^v/, '')

  const revision = v.match(/-r(\d+)$/)
  if (revision) {
    parsed.revision = parseInt(revision[1], 10)
    v = v.slice(0, revision.index)
  } else {
    v = v.replace(/-/g, '.')
  }

  let head = v
  const underscore = v.indexOf('_')
  if (underscore >= 0) {
    head = v.slice(0, underscore)
    for (const part of v.slice(underscore + 1).split('_')) {
      const name = part.replace(/\d+$/, '')
      parsed.suffixes.push({
        rank: name in SUFFIX_RANK ? SUFFIX_RANK[name] : NO_SUFFIX,
        number: leadingInt(part.slice(name.length)),
      })
    }
  }

  for (const part of head.split('.')) {
    const digits = part.replace(/[a-z]+$/, '')
    parsed.numbers.push(leadingInt(digits))
    if (digits.length < part.length) parsed.letter = part.charCodeAt(digits.length)
  }

  return parsed
}

function sign(n: number): number {
  return n > 0 ? 1 : n < 0 ? -1 : 0
}

export function compareVersions(a: string, b: string): number {
  const pa = parse(a)
  const pb = parse(b)

  for (let i = 0; i < Math.max(pa.numbers.length, pb.numbers.length); i++) {
    const c = sign((pa.numbers[i] ?? 0) - (pb.numbers[i] ?? 0))
    if (c) return c
  }

  const letter = sign(pa.letter - pb.letter)
  if (letter) return letter

  for (let i = 0; i < Math.max(pa.suffixes.length, pb.suffixes.length); i++) {
    const as = pa.suffixes[i] ?? { rank: NO_SUFFIX, number: 0 }
    const bs = pb.suffixes[i] ?? { rank: NO_SUFFIX, number: 0 }
    const rank = sign(as.rank - bs.rank)
    if (rank) return rank
    const number = sign(as.number - bs.number)
    if (number) return number
  }

  return sign(pa.revision - pb.revision)
}
