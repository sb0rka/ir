import {
  clampQueueLimit,
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
    if ('|()[],:*'.includes(char)) {
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
  private readonly aliases = new Map<string, { field: string; aggregate?: AggregateFn }>()
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
    const name = this.expectIdent('ожидалась стадия filter, group, select, sort или limit').toLowerCase()
    if (name !== 'filter' && name !== 'group' && name !== 'select' && name !== 'sort' && name !== 'limit') {
      throw new ParseFailure(`неизвестная стадия ${name}`, this.prev().start)
    }
    if (name !== 'sort' && seen.has(name)) {
      throw new ParseFailure(`стадия ${name} повторяется`, this.prev().start)
    }
    seen.add(name)
    this.expectPunct('(', `ожидалась ( после ${name}`)
    if (name === 'filter') this.parseFilter(ast)
    if (name === 'group') this.parseGroup(ast)
    if (name === 'select') this.parseSelect(ast)
    if (name === 'sort') this.parseSort(ast)
    if (name === 'limit') this.parseLimit(ast)
    this.expectPunct(')', `ожидалась ) после ${name}`)
  }

  private parseFilter(ast: QueryAst) {
    ast.filter.push(this.parseCondition())
    while (this.peekKeyword('and') || this.peekKeyword('or')) {
      ast.joiners.push(this.take().value.toLowerCase() as LogicalJoiner)
      ast.filter.push(this.parseCondition())
    }
  }

  private parseCondition(): Condition {
    const negated = this.tryKeyword('not')
    const field = this.expectIdent('ожидалось имя поля')
    if (this.tryKeyword('is')) {
      const notNull = this.tryKeyword('not')
      this.expectKeyword('null', 'ожидалось null')
      return {
        id: newId('cond'),
        field,
        op: notNull ? 'is_not_null' : 'is_null',
        value: '',
        values: [],
        negated,
      }
    }
    if (this.tryKeyword('in')) {
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
    if (this.peekKeyword('contains') || this.peekKeyword('startswith')) {
      return this.take().value.toLowerCase() as CompareOp
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
    if (this.peekNamedGroupArg()) {
      this.parseNamedGroup(ast)
      return
    }
    ast.groups.push({ id: newId('grp'), field: this.expectIdent('ожидалось поле группировки') })
    while (this.tryPunct(',')) {
      ast.groups.push({ id: newId('grp'), field: this.expectIdent('ожидалось поле группировки') })
    }
  }

  private peekNamedGroupArg(): boolean {
    if (!this.peekKeyword('key') && !this.peekKeyword('agg')) return false
    const next = this.tokens[this.pos + 1]
    return next?.kind === 'punct' && next.value === ':'
  }

  private parseNamedGroup(ast: QueryAst) {
    const seen = new Set<string>()
    while (!this.peekPunct(')')) {
      const name = this.expectIdent('ожидался параметр key или agg').toLowerCase()
      this.expectPunct(':', `ожидалось : после ${name}`)
      if (name !== 'key' && name !== 'agg') {
        throw new ParseFailure(`неизвестный параметр ${name}`, this.prev().start)
      }
      if (seen.has(name)) {
        throw new ParseFailure(`параметр ${name} повторяется`, this.prev().start)
      }
      seen.add(name)
      if (name === 'key') {
        for (const field of this.parseGroupKeyList()) {
          if (ast.groups.some((group) => group.field === field)) continue
          ast.groups.push({ id: newId('grp'), field })
        }
      } else {
        this.parseGroupAggList(ast)
      }
      this.tryPunct(',')
    }
  }

  private parseGroupKeyList(): string[] {
    const bracketed = this.tryPunct('[')
    const fields: string[] = []
    if (!this.peekPunct(']') && !this.peekPunct(')') && !this.peekNamedGroupArg()) {
      fields.push(this.expectIdent('ожидалось поле группировки'))
      while (this.tryPunct(',')) {
        if (this.peekPunct(']') || this.peekPunct(')') || this.peekNamedGroupArg()) break
        fields.push(this.expectIdent('ожидалось поле группировки'))
      }
    }
    if (bracketed) this.expectPunct(']', 'ожидалась ] после списка key')
    return fields
  }

  private parseGroupAggList(ast: QueryAst) {
    this.mergeAggregateColumn(ast, this.parseSelectItem())
    while (this.tryPunct(',')) {
      if (this.peekPunct(')') || this.peekNamedGroupArg()) break
      this.mergeAggregateColumn(ast, this.parseSelectItem())
    }
  }

  private mergeAggregateColumn(ast: QueryAst, column: Column) {
    if (!column.aggregate) {
      throw new ParseFailure('ожидалась агрегатная функция', this.prev().start)
    }
    this.mergeColumn(ast, column)
  }

  private mergeColumn(ast: QueryAst, column: Column): Column {
    const match = ast.columns.find(
      (item) => item.field === column.field && item.aggregate === column.aggregate,
    )
    if (!match) {
      ast.columns.push(column)
      return column
    }
    if (column.sort && !match.sort) match.sort = column.sort
    return match
  }

  private parseSelect(ast: QueryAst) {
    if (this.peekPunct(')')) return
    this.mergeColumn(ast, this.parseSelectItem())
    while (this.tryPunct(',')) this.mergeColumn(ast, this.parseSelectItem())
  }

  private parseSelectItem(): Column {
    const name = this.expectIdent('ожидалось поле или агрегат')
    if (this.tryPunct('(')) {
      const fn = name.toLowerCase()
      if (!AGGREGATE_SET.has(fn)) {
        throw new ParseFailure(`неизвестная агрегатная функция ${name}`, this.prev().start)
      }
      let field = ''
      if (this.tryPunct('*')) {
        field = ''
      } else if (!this.peekPunct(')')) {
        field = this.expectIdent('ожидалось поле агрегата')
      }
      this.expectPunct(')', 'ожидалась ) после агрегата')
      const column: Column = { id: newId('col'), field, aggregate: fn as AggregateFn }
      this.finishAlias(column)
      return column
    }
    const alias = this.aliases.get(name)
    if (alias) {
      const column: Column = { id: newId('col'), field: alias.field, aggregate: alias.aggregate }
      this.finishAlias(column)
      return column
    }
    const column: Column = { id: newId('col'), field: name }
    this.finishAlias(column)
    return column
  }

  private finishAlias(column: Column) {
    if (!this.tryKeyword('as')) return
    const alias = this.expectIdent('ожидался псевдоним')
    this.aliases.set(alias, { field: column.field, aggregate: column.aggregate })
  }

  private parseSort(ast: QueryAst) {
    if (this.peekPunct(')')) return
    let priority = ast.columns.reduce((max, column) => Math.max(max, column.sort?.priority ?? 0), 0)
    this.applySort(ast, this.parseSortItem(), (priority += 1))
    while (this.tryPunct(',')) {
      this.applySort(ast, this.parseSortItem(), (priority += 1))
    }
  }

  private parseSortItem(): { column: Column; dir: SortDir } {
    const column = this.parseSelectItem()
    let dir: SortDir = 'asc'
    if (this.peekKeyword('asc') || this.peekKeyword('desc')) {
      dir = this.take().value.toLowerCase() as SortDir
    }
    return { column, dir }
  }

  private applySort(ast: QueryAst, item: { column: Column; dir: SortDir }, priority: number) {
    const target = this.mergeColumn(ast, item.column)
    target.sort = { dir: item.dir, priority }
  }

  private parseLimit(ast: QueryAst) {
    const token = this.peek()
    if (token.kind !== 'number' || !/^\d+$/.test(token.value)) {
      throw new ParseFailure('ожидалось целое число', token.start)
    }
    const value = Number(token.value)
    if (value < 1) throw new ParseFailure('limit должен быть не меньше 1', token.start)
    this.take()
    // SIEM PDQL pasted from MaxPatrol may carry limit(10000); the queue caps it
    // and the canonical PDQL shows the value actually applied.
    ast.limit = clampQueueLimit(value)
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

  private peekKeyword(value: string): boolean {
    const token = this.peek()
    return token.kind === 'ident' && token.value.toLowerCase() === value
  }

  private tryKeyword(value: string): boolean {
    if (!this.peekKeyword(value)) return false
    this.take()
    return true
  }

  private expectKeyword(value: string, message: string) {
    if (!this.tryKeyword(value)) throw new ParseFailure(message, this.peek().start)
  }

  private peekPunct(value: string): boolean {
    const token = this.peek()
    return token.kind === 'punct' && token.value === value
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
