import {
  emptyQuery,
  newId,
  type AggregateFn,
  type Column,
  type CompareOp,
  type Condition,
  type LogicalJoiner,
  type ParseResult,
  type QueryAst,
  type SortDir,
} from './model'

const AGGREGATE_SET = new Set<string>(['count', 'uniq', 'min', 'max', 'avg'])
const COMPARE_OPS = new Set<string>(['=', '!=', '>', '<', '>=', '<=', 'contains', 'startswith'])

type TokenKind = 'ident' | 'string' | 'number' | 'op' | 'punct' | 'eof'
interface Token {
  kind: TokenKind
  value: string
  start: number
}

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
    const char = input[i]
    if (/\s/.test(char)) {
      i += 1
      continue
    }
    if (char === '"') {
      let value = ''
      i += 1
      while (i < input.length) {
        const next = input[i]
        if (next === '"') {
          i += 1
          tokens.push({ kind: 'string', value, start })
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
      if (tokens.at(-1)?.start !== start) {
        throw new ParseFailure('незакрытая строка', start)
      }
      continue
    }
    if (char === '|' || char === '(' || char === ')' || char === ',') {
      tokens.push({ kind: 'punct', value: char, start })
      i += 1
      continue
    }
    if (char === '>' || char === '<' || char === '!' || char === '=') {
      if ((char === '>' || char === '<' || char === '!') && input[i + 1] === '=') {
        tokens.push({ kind: 'op', value: `${char}=`, start })
        i += 2
        continue
      }
      if (char === '!') {
        throw new ParseFailure('ожидался оператор !=', start)
      }
      tokens.push({ kind: 'op', value: char, start })
      i += 1
      continue
    }
    if (/[0-9]/.test(char) || (char === '-' && /[0-9]/.test(input[i + 1] ?? ''))) {
      let j = i + 1
      while (j < input.length && /[0-9]/.test(input[j])) j += 1
      if (input[j] === '.' && /[0-9]/.test(input[j + 1] ?? '')) {
        j += 1
        while (j < input.length && /[0-9]/.test(input[j])) j += 1
      }
      tokens.push({ kind: 'number', value: input.slice(i, j), start })
      i = j
      continue
    }
    if (/[A-Za-z_]/.test(char)) {
      let j = i + 1
      while (j < input.length && /[A-Za-z0-9_.]/.test(input[j])) j += 1
      tokens.push({ kind: 'ident', value: input.slice(i, j), start })
      i = j
      continue
    }
    throw new ParseFailure(`неожиданный символ ${JSON.stringify(char)}`, start)
  }
  tokens.push({ kind: 'eof', value: '', start: input.length })
  return tokens
}

class Parser {
  private readonly tokens: Token[]
  private pos = 0

  constructor(tokens: Token[]) {
    this.tokens = tokens
  }

  parse(): QueryAst {
    if (this.peek().kind === 'eof') return emptyQuery()
    const ast = emptyQuery()
    const seen = new Set<string>()
    this.parseStage(ast, seen)
    while (this.tryPunct('|')) {
      this.parseStage(ast, seen)
    }
    this.expect('eof', 'лишние символы после запроса')
    return ast
  }

  private parseStage(ast: QueryAst, seen: Set<string>) {
    const name = this.expectIdent('ожидалась стадия filter, group, select или sort')
    if (seen.has(name)) {
      throw new ParseFailure(`стадия ${name} повторяется`, this.prev().start)
    }
    if (name !== 'filter' && name !== 'group' && name !== 'select' && name !== 'sort') {
      throw new ParseFailure(`неизвестная стадия ${name}`, this.prev().start)
    }
    seen.add(name)
    this.expectPunct('(', `ожидалась ( после ${name}`)
    if (name === 'filter') this.parseFilter(ast)
    if (name === 'group') this.parseGroup(ast)
    if (name === 'select') this.parseSelect(ast)
    if (name === 'sort') this.parseSort(ast)
    this.expectPunct(')', `ожидалась ) после ${name}`)
  }

  private parseFilter(ast: QueryAst) {
    ast.filter.push(this.parseCondition())
    while (this.peekIdent('and') || this.peekIdent('or')) {
      ast.joiners.push(this.take().value as LogicalJoiner)
      ast.filter.push(this.parseCondition())
    }
  }

  private parseCondition(): Condition {
    const negated = this.tryIdent('not')
    const field = this.expectIdent('ожидалось имя поля')
    if (this.tryIdent('is')) {
      const notNull = this.tryIdent('not')
      this.expectIdentValue('null', 'ожидалось null')
      return {
        id: newId('cond'),
        field,
        op: notNull ? 'is_not_null' : 'is_null',
        value: '',
        values: [],
        negated,
      }
    }
    if (this.tryIdent('in')) {
      this.expectPunct('(', 'ожидалась ( после in')
      const values: string[] = []
      if (!this.peekPunct(')')) {
        values.push(this.parseLiteral())
        while (this.tryPunct(',')) values.push(this.parseLiteral())
      }
      this.expectPunct(')', 'ожидалась ) после списка in')
      return { id: newId('cond'), field, op: 'in', value: '', values, negated }
    }
    const op = this.parseCompareOp()
    const value = this.parseLiteral()
    return { id: newId('cond'), field, op, value, values: [], negated }
  }

  private parseCompareOp(): CompareOp {
    const token = this.peek()
    if (token.kind === 'op' && COMPARE_OPS.has(token.value)) {
      this.take()
      return token.value as CompareOp
    }
    if (token.kind === 'ident' && (token.value === 'contains' || token.value === 'startswith')) {
      this.take()
      return token.value
    }
    throw new ParseFailure('ожидался оператор сравнения', token.start)
  }

  private parseLiteral(): string {
    const token = this.peek()
    if (token.kind === 'string' || token.kind === 'number') {
      this.take()
      return token.value
    }
    throw new ParseFailure('ожидалось значение в кавычках или число', token.start)
  }

  private parseGroup(ast: QueryAst) {
    if (this.peekPunct(')')) return
    ast.groups.push({ id: newId('grp'), field: this.expectIdent('ожидалось поле группировки') })
    while (this.tryPunct(',')) {
      ast.groups.push({ id: newId('grp'), field: this.expectIdent('ожидалось поле группировки') })
    }
  }

  private parseSelect(ast: QueryAst) {
    if (this.peekPunct(')')) return
    ast.columns.push(this.parseSelectItem())
    while (this.tryPunct(',')) ast.columns.push(this.parseSelectItem())
  }

  private parseSelectItem(): Column {
    const name = this.expectIdent('ожидалось поле или агрегат')
    if (this.tryPunct('(')) {
      if (!AGGREGATE_SET.has(name)) {
        throw new ParseFailure(`неизвестная агрегатная функция ${name}`, this.prev().start)
      }
      const field = this.peekPunct(')') ? '' : this.expectIdent('ожидалось поле агрегата')
      this.expectPunct(')', 'ожидалась ) после агрегата')
      return { id: newId('col'), field, aggregate: name as AggregateFn }
    }
    return { id: newId('col'), field: name }
  }

  private parseSort(ast: QueryAst) {
    if (this.peekPunct(')')) return
    let priority = 1
    this.applySort(ast, this.parseSortItem(), priority)
    while (this.tryPunct(',')) {
      priority += 1
      this.applySort(ast, this.parseSortItem(), priority)
    }
  }

  private parseSortItem(): { column: Column; dir: SortDir } {
    const column = this.parseSelectItem()
    let dir: SortDir = 'asc'
    if (this.peekIdent('asc') || this.peekIdent('desc')) {
      dir = this.take().value as SortDir
    }
    return { column, dir }
  }

  private applySort(ast: QueryAst, item: { column: Column; dir: SortDir }, priority: number) {
    const match = ast.columns.find(
      (column) => column.field === item.column.field && column.aggregate === item.column.aggregate,
    )
    const target = match ?? item.column
    target.sort = { dir: item.dir, priority }
    if (!match) ast.columns.push(target)
  }

  private peek(): Token {
    return this.tokens[this.pos] ?? this.tokens[this.tokens.length - 1]
  }

  private prev(): Token {
    return this.tokens[Math.max(0, this.pos - 1)]
  }

  private take(): Token {
    const token = this.peek()
    this.pos += 1
    return token
  }

  private peekIdent(value: string): boolean {
    const token = this.peek()
    return token.kind === 'ident' && token.value === value
  }

  private peekPunct(value: string): boolean {
    const token = this.peek()
    return token.kind === 'punct' && token.value === value
  }

  private tryIdent(value: string): boolean {
    if (!this.peekIdent(value)) return false
    this.take()
    return true
  }

  private tryPunct(value: string): boolean {
    if (!this.peekPunct(value)) return false
    this.take()
    return true
  }

  private expectIdent(message: string): string {
    const token = this.peek()
    if (token.kind !== 'ident') throw new ParseFailure(message, token.start)
    return this.take().value
  }

  private expectIdentValue(value: string, message: string) {
    const token = this.peek()
    if (token.kind !== 'ident' || token.value !== value) {
      throw new ParseFailure(message, token.start)
    }
    this.take()
  }

  private expectPunct(value: string, message: string) {
    if (!this.tryPunct(value)) throw new ParseFailure(message, this.peek().start)
  }

  private expect(kind: TokenKind, message: string) {
    const token = this.peek()
    if (token.kind !== kind) throw new ParseFailure(message, token.start)
    this.take()
  }
}

export function parse(input: string): ParseResult {
  try {
    const ast = new Parser(tokenize(input.trim())).parse()
    return { ok: true, ast }
  } catch (err) {
    if (err instanceof ParseFailure) {
      return { ok: false, error: { message: err.message, position: err.position } }
    }
    throw err
  }
}
