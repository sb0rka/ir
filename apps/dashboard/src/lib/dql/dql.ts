import {
  isFilterGroup,
  newId,
  type CompareOp,
  type Condition,
  type FilterGroup,
  type FilterNode,
  type LogicalJoiner,
  type ParseError,
} from '../pdql/model'

export type DqlParseResult =
  | { ok: true; filter: FilterNode[]; joiners: LogicalJoiner[] }
  | { ok: false; error: ParseError }

const SAFE_WILDCARD = /^[A-Za-z0-9_.:@/-]+$/

function quote(value: string): string {
  return `"${value.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`
}

function isNumericLiteral(value: string): boolean {
  return /^-?\d+(\.\d+)?$/.test(value)
}

function eqLiteral(value: string): string {
  return isNumericLiteral(value) ? value : quote(value)
}

function formatPredicate(condition: Condition): string | null {
  const field = condition.field
  switch (condition.op) {
    case 'is_null':
      return condition.negated ? `${field}:*` : `NOT ${field}:*`
    case 'is_not_null':
      return condition.negated ? `NOT ${field}:*` : `${field}:*`
    case 'contains':
      if (!SAFE_WILDCARD.test(condition.value)) return null
      return condition.negated ? `NOT ${field}:*${condition.value}*` : `${field}:*${condition.value}*`
    case 'in': {
      const list = condition.values.map(eqLiteral).join(' OR ')
      const text = `${field}:(${list})`
      return condition.negated ? `NOT ${text}` : text
    }
    case '>':
    case '>=':
    case '<':
    case '<=': {
      const text = `${field} ${condition.op} ${condition.value}`
      return condition.negated ? `NOT (${text})` : text
    }
    case '=': {
      const text = `${field}:${eqLiteral(condition.value)}`
      return condition.negated ? `NOT ${text}` : text
    }
    case '!=': {
      const text = `NOT ${field}:${eqLiteral(condition.value)}`
      return condition.negated ? `${field}:${eqLiteral(condition.value)}` : text
    }
    default:
      return null
  }
}

function formatNode(node: FilterNode): string | null {
  if (isFilterGroup(node)) {
    const inner = formatList(node.children, node.joiners)
    if (inner == null) return null
    return node.negated ? `NOT (${inner})` : `(${inner})`
  }
  return formatPredicate(node)
}

function formatList(nodes: FilterNode[], joiners: LogicalJoiner[]): string | null {
  if (nodes.length === 0) return ''
  const parts: string[] = []
  for (const node of nodes) {
    const text = formatNode(node)
    if (text == null) return null
    parts.push(text)
  }
  const mixed = joiners.some((joiner) => joiner !== (joiners[0] ?? 'and'))
  if (!mixed) {
    const word = (joiners[0] ?? 'and').toUpperCase()
    return parts.join(` ${word} `)
  }
  let text = parts[0] ?? ''
  for (let index = 1; index < parts.length; index++) {
    const word = (joiners[index - 1] ?? 'and').toUpperCase()
    text = index === 1 ? `${text} ${word} ${parts[index]}` : `(${text}) ${word} ${parts[index]}`
  }
  return text
}

/** Bounded Wazuh DQL predicate. Null when a condition cannot be represented without changing its meaning. */
export function formatDql(filter: FilterNode[], joiners: LogicalJoiner[]): string | null {
  return formatList(filter, joiners)
}

type Token =
  | { kind: 'ident'; value: string; start: number }
  | { kind: 'string'; value: string; start: number }
  | { kind: 'number'; value: string; start: number }
  | { kind: 'op'; value: string; start: number }
  | { kind: 'punct'; value: string; start: number }
  | { kind: 'kw'; value: 'and' | 'or' | 'not'; start: number }

class ParseFailure extends Error {
  position: number
  constructor(message: string, position: number) {
    super(message)
    this.position = position
  }
}

function tokenize(input: string): Token[] {
  const tokens: Token[] = []
  let i = 0
  while (i < input.length) {
    const start = i
    const char = input[i] ?? ''
    if (/\s/.test(char)) {
      i += 1
      continue
    }
    if (char === '"') {
      let value = ''
      i += 1
      let closed = false
      while (i < input.length) {
        const next = input[i] ?? ''
        if (next === '"') {
          i += 1
          closed = true
          break
        }
        if (next === '\\' && i + 1 < input.length) {
          value += input[i + 1]
          i += 2
          continue
        }
        value += next
        i += 1
      }
      if (!closed) throw new ParseFailure('незакрытая строка', start)
      tokens.push({ kind: 'string', value, start })
      continue
    }
    if ('():'.includes(char)) {
      tokens.push({ kind: 'punct', value: char, start })
      i += 1
      continue
    }
    if (char === '>' || char === '<') {
      if (input[i + 1] === '=') {
        tokens.push({ kind: 'op', value: `${char}=`, start })
        i += 2
        continue
      }
      tokens.push({ kind: 'op', value: char, start })
      i += 1
      continue
    }
    if (/[0-9*]/.test(char) || (char === '-' && /[0-9]/.test(input[i + 1] ?? ''))) {
      let j = i + 1
      while (j < input.length && /[0-9A-Za-z_.:@/*-]/.test(input[j] ?? '')) j += 1
      const value = input.slice(i, j)
      if (/^-?\d+(\.\d+)?$/.test(value)) tokens.push({ kind: 'number', value, start })
      else tokens.push({ kind: 'ident', value, start })
      i = j
      continue
    }
    if (/[A-Za-z_]/.test(char)) {
      let j = i + 1
      while (j < input.length && /[A-Za-z0-9_.]/.test(input[j] ?? '')) j += 1
      const value = input.slice(i, j)
      const word = value.toLowerCase()
      if (word === 'and' || word === 'or' || word === 'not') tokens.push({ kind: 'kw', value: word, start })
      else tokens.push({ kind: 'ident', value, start })
      i = j
      continue
    }
    throw new ParseFailure('неподдерживаемый символ DQL', start)
  }
  return tokens
}

class Parser {
  private index = 0
  private readonly tokens: Token[]

  constructor(tokens: Token[]) {
    this.tokens = tokens
  }

  parse(): { filter: FilterNode[]; joiners: LogicalJoiner[] } {
    if (this.tokens.length === 0) return { filter: [], joiners: [] }
    const node = this.parseOr()
    if (this.peek()) throw new ParseFailure('лишний фрагмент DQL', this.peek()?.start ?? 0)
    if (isFilterGroup(node) && !node.negated) return { filter: node.children, joiners: node.joiners }
    return { filter: [node], joiners: [] }
  }

  private peek(): Token | undefined {
    return this.tokens[this.index]
  }

  private pull(): Token {
    const token = this.tokens[this.index]
    if (!token) throw new ParseFailure('неожиданный конец DQL', 0)
    this.index += 1
    return token
  }

  private parseOr(): FilterNode {
    const children = [this.parseAnd()]
    const joiners: LogicalJoiner[] = []
    while (this.peek()?.kind === 'kw' && this.peek()?.value === 'or') {
      this.pull()
      children.push(this.parseAnd())
      joiners.push('or')
    }
    if (children.length === 1) return children[0]!
    return { kind: 'group', id: newId('dql'), negated: false, children, joiners }
  }

  private parseAnd(): FilterNode {
    const children = [this.parseNot()]
    const joiners: LogicalJoiner[] = []
    while (this.peek()?.kind === 'kw' && this.peek()?.value === 'and') {
      this.pull()
      children.push(this.parseNot())
      joiners.push('and')
    }
    if (children.length === 1) return children[0]!
    return { kind: 'group', id: newId('dql'), negated: false, children, joiners }
  }

  private parseNot(): FilterNode {
    if (this.peek()?.kind === 'kw' && this.peek()?.value === 'not') {
      this.pull()
      const node = this.parseNot()
      if (!isFilterGroup(node) && node.op === 'is_not_null' && !node.negated) {
        return { ...node, op: 'is_null' }
      }
      if (!isFilterGroup(node) && node.op === '=' && !node.negated) {
        return { ...node, negated: true }
      }
      if (isFilterGroup(node)) return { ...node, negated: !node.negated }
      return { ...node, negated: !node.negated }
    }
    return this.parsePrimary()
  }

  private parsePrimary(): FilterNode {
    if (this.peek()?.kind === 'punct' && this.peek()?.value === '(') {
      this.pull()
      const node = this.parseOr()
      const close = this.pull()
      if (close.kind !== 'punct' || close.value !== ')') {
        throw new ParseFailure('ожидалась закрывающая скобка', close.start)
      }
      if (isFilterGroup(node)) return node
      return { kind: 'group', id: newId('dql'), negated: false, children: [node], joiners: [] }
    }
    return this.parsePredicate()
  }

  private parsePredicate(): Condition {
    const fieldToken = this.pull()
    if (fieldToken.kind !== 'ident') throw new ParseFailure('ожидалось поле', fieldToken.start)
    const next = this.peek()
    if (next?.kind === 'op') {
      const op = this.pull().value as CompareOp
      const value = this.parseLiteral()
      return condition(fieldToken.value, op, value, [])
    }
    if (next?.kind !== 'punct' || next.value !== ':') {
      throw new ParseFailure('ожидалось : или оператор сравнения', next?.start ?? fieldToken.start)
    }
    this.pull()
    if (this.peek()?.kind === 'punct' && this.peek()?.value === '(') {
      this.pull()
      const values: string[] = []
      values.push(this.parseLiteral())
      while (this.peek()?.kind === 'kw' && this.peek()?.value === 'or') {
        this.pull()
        values.push(this.parseLiteral())
      }
      const close = this.pull()
      if (close.kind !== 'punct' || close.value !== ')') {
        throw new ParseFailure('ожидалась закрывающая скобка списка', close.start)
      }
      return condition(fieldToken.value, 'in', '', values)
    }
    const literal = this.parseRawLiteral()
    if (literal === '*') return condition(fieldToken.value, 'is_not_null', '', [])
    if (literal.startsWith('*') && literal.endsWith('*') && literal.length > 2 && !literal.slice(1, -1).includes('*')) {
      return condition(fieldToken.value, 'contains', literal.slice(1, -1), [])
    }
    return condition(fieldToken.value, '=', literal, [])
  }

  private parseLiteral(): string {
    const token = this.pull()
    if (token.kind === 'string' || token.kind === 'number' || token.kind === 'ident') return token.value
    throw new ParseFailure('ожидалось значение', token.start)
  }

  private parseRawLiteral(): string {
    return this.parseLiteral()
  }
}

function condition(field: string, op: CompareOp, value: string, values: string[]): Condition {
  return { id: newId('dql'), field, op, value, values, negated: false }
}

export function parseDql(input: string): DqlParseResult {
  try {
    const parsed = new Parser(tokenize(input.trim())).parse()
    return { ok: true, ...parsed }
  } catch (err) {
    if (err instanceof ParseFailure) return { ok: false, error: { message: err.message, position: err.position } }
    throw err
  }
}

export function isFilterGroupNode(node: FilterNode): node is FilterGroup {
  return isFilterGroup(node)
}
