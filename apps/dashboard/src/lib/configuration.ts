const MAX_CONFIGURATION_BYTES = 64 * 1024

/** Index sources without silently accepting duplicate JSON keys or source names. */
export function parseConfiguration(text: string): Record<string, Record<string, string>> {
  let offset = 0
  let whitespaceBytes = 0
  const whitespace = () => { while (/[\t\n\r ]/.test(text[offset] ?? '\u0000')) { offset++; whitespaceBytes++ } }
  const string = (): string => {
    whitespace()
    const start = offset
    if (text[offset++] !== '"') throw new Error('Ключи и значения JSON должны быть строками')
    while (offset < text.length) {
      const character = text[offset++]
      if (character === '\\') { offset++; continue }
      if (character === '"') {
        try { return JSON.parse(text.slice(start, offset)) as string }
        catch { throw new Error('Некорректная JSON-строка') }
      }
    }
    throw new Error('Незавершённая JSON-строка')
  }
  const sources: Record<string, Record<string, string>> = Object.create(null) as Record<string, Record<string, string>>
  whitespace()
  if (text[offset++] !== '[') throw new Error('Введите JSON-массив источников: [{"name":"источник","ключ":"значение"}]')
  whitespace()
  if (text[offset] !== ']') {
    for (;;) {
      whitespace()
      if (text[offset++] !== '{') throw new Error('Каждый источник должен быть JSON-объектом')
      const values: Record<string, string> = Object.create(null) as Record<string, string>
      whitespace()
      if (text[offset] !== '}') {
        for (;;) {
          const key = string()
          if (Object.hasOwn(values, key)) throw new Error('JSON содержит повторяющийся ключ')
          whitespace()
          if (text[offset++] !== ':') throw new Error('После ключа ожидается двоеточие')
          values[key] = string()
          whitespace()
          if (text[offset] !== ',') break
          offset++
        }
      }
      if (text[offset++] !== '}') throw new Error('Некорректный JSON-объект источника')
      const name = values.name
      if (!name?.trim()) throw new Error('У каждого источника должно быть непустое строковое поле name')
      if (Object.hasOwn(sources, name)) throw new Error('JSON содержит повторяющееся имя источника')
      delete values.name
      sources[name] = values
      whitespace()
      if (text[offset] !== ',') break
      offset++
    }
  }
  if (text[offset++] !== ']') throw new Error('Некорректный JSON-массив')
  whitespace()
  if (offset !== text.length) throw new Error('После JSON-массива есть лишние данные')
  if (new TextEncoder().encode(text).length - whitespaceBytes > MAX_CONFIGURATION_BYTES) throw new Error('JSON-конфигурация превышает 64 КиБ')
  return sources
}
