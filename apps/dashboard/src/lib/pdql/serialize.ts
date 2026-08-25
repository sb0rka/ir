import { isGroupDimensionColumn, type Column, type CompareOp, type Condition, type QueryAst } from './model'

function quote(value: string): string {
  return `"${value.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`
}

function isNumericLiteral(value: string): boolean {
  return /^-?\d+(\.\d+)?$/.test(value)
}

function formatValue(value: string): string {
  return isNumericLiteral(value) ? value : quote(value)
}

function formatSelectItem(column: Column): string {
  if (column.aggregate) {
    return column.field ? `${column.aggregate}(${column.field})` : `${column.aggregate}()`
  }
  return column.field
}

function formatCondition(ast: QueryAst): string {
  return ast.filter
    .map((condition, index) => {
      const body = formatConditionLabel(condition)
      const joiner = index === 0 ? '' : ` ${ast.joiners[index - 1] ?? 'and'} `
      return `${joiner}${body}`
    })
    .join('')
}

export function formatConditionLabel(condition: Condition): string {
  const prefix = condition.negated ? 'not ' : ''
  return `${prefix}${formatPredicate(condition.field, condition.op, condition.value, condition.values)}`
}

function formatPredicate(field: string, op: CompareOp, value: string, values: string[]): string {
  switch (op) {
    case 'is_null':
      return `${field} is null`
    case 'is_not_null':
      return `${field} is not null`
    case 'in':
      return `${field} in (${values.map(formatValue).join(', ')})`
    case 'contains':
    case 'startswith':
      return `${field} ${op} ${formatValue(value)}`
    default:
      return `${field} ${op} ${formatValue(value)}`
  }
}

export function serialize(ast: QueryAst): string {
  const stages: string[] = []
  if (ast.filter.length > 0) {
    stages.push(`filter(${formatCondition(ast)})`)
  }
  if (ast.groups.length > 0) {
    stages.push(`group(${ast.groups.map((group) => group.field).join(', ')})`)
  }
  const selectItems = [
    ...ast.groups.map((group) => group.field),
    ...ast.columns.filter((column) => !isGroupDimensionColumn(ast, column)).map(formatSelectItem),
  ]
  if (selectItems.length > 0) {
    stages.push(`select(${selectItems.join(', ')})`)
  }
  const sorted = ast.columns
    .filter((column) => column.sort)
    .slice()
    .sort((left, right) => (left.sort?.priority ?? 0) - (right.sort?.priority ?? 0))
  if (sorted.length > 0) {
    stages.push(
      `sort(${sorted.map((column) => `${formatSelectItem(column)} ${column.sort?.dir ?? 'asc'}`).join(', ')})`,
    )
  }
  return stages.join(' | ')
}
