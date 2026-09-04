/**
 * Display names for MaxPatrol incident type codes
 * (`/api/incident_management/v1/dictionaries/incidents/types`).
 * Gateway returns the leaf `type` code; SIEM has no i18n on the finding itself.
 */
const INCIDENT_TYPE_LABELS_RU: Record<string, string> = {
  NotDefined: 'Не определен',
  DoSAttack: 'DDoS-атака',
  LoginAttempt: 'Неудачные попытки авторизации',
  InfectionAttempt: 'Попытки внедрения ВПО',
  ExploitAttempt: 'Попытки эксплуатации уязвимости',
  Phishing: 'Публикация мошеннической информации',
  Scanning: 'Сетевое сканирование',
  SocialEngineering: 'Социальная инженерия',
  Attack: 'Компьютерная атака',
  MalwareInfection: 'Заражение ВПО',
  DoS: 'Замедление работы ресурса в результате DDoS-атаки',
  TrafficHijacking: 'Захват сетевого трафика',
  AccountCompromise: 'Компрометация учетной записи',
  UnauthorisedModification: 'Несанкционированное изменение информации',
  UnauthorisedAccess: 'Несанкционированное разглашение информации',
  AbusiveContent: 'Распространение оскорбительной информации',
  ApplicationCompromise: 'Успешная эксплуатация уязвимости',
  MalwareCommandAndControl: 'Использование контролируемого ресурса для проведения атак',
  Fraud: 'Несанкционированное использование ресурса',
  NotAComputerAttack: 'Событие не связано с компьютерной атакой',
  InformationLeakage: 'Утечка информации',
  Incident: 'Компьютерный инцидент',
  ProhibitedServicesUsage: 'Использование запрещенных сервисов',
  UpdatesViolation: 'Нарушение регламента по установке обновлений',
  PermissionViolation: 'Превышение служебных полномочий',
  ApplicationViolation: 'Установка запрещенного ПО',
  AnotherPolicyViolation: 'Другое нарушение политик ИБ',
  PolicyViolation: 'Нарушение политик ИБ',
  Vulnerability: 'Выявление уязвимости',
  VulnerabilityDetection: 'Выявление уязвимостей',
}

/** Resolve MaxPatrol incident type code to a Russian display name. */
export function incidentTypeLabelRu(type: string | undefined | null): string {
  const raw = (type ?? '').trim()
  if (!raw) return ''
  const leaf = raw.includes('.') ? raw.slice(raw.lastIndexOf('.') + 1) : raw
  return INCIDENT_TYPE_LABELS_RU[leaf] ?? INCIDENT_TYPE_LABELS_RU[raw] ?? raw
}
