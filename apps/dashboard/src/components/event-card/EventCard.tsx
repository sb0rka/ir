import { useState } from 'react'
import type { TimeInterval } from '../time-interval'
import { EventFieldModal } from './EventFieldModal'
import { EventFields, EventHeaderFacts, type EventCardModel } from './EventFields'
import { EventTimeButton } from './EventTimeButton'

export function EventCard({
  event,
  investigationId,
  eventInContext = false,
  timeInterval,
  onTimeChange,
  onAddFilter,
  onAddToContext,
}: {
  event: EventCardModel
  investigationId?: string
  eventInContext?: boolean
  timeInterval: TimeInterval
  onTimeChange: (value: TimeInterval) => void
  onAddFilter: (field: string, value: string) => void
  onAddToContext?: (field: string, value: string, includeEvent: boolean) => Promise<void>
}) {
  const [picked, setPicked] = useState<{ field: string; value: string } | null>(null)
  const raw = event.raw ?? {}

  return (
    <div className="space-y-3">
      <div className="space-y-1.5">
        <EventTimeButton time={event.time} current={timeInterval} onChange={onTimeChange} />
        <div className="text-sm font-medium leading-snug">{event.title}</div>
        {event.description && event.description !== event.title && (
          <p className="text-xs leading-relaxed text-fg-muted">{event.description}</p>
        )}
      </div>
      <EventHeaderFacts
        source={event.source}
        raw={raw}
        severity={event.severity}
        onValueClick={(field, value) => setPicked({ field, value })}
      />
      <EventFields
        source={event.source}
        raw={raw}
        onValueClick={(field, value) => setPicked({ field, value })}
      />
      {picked && (
        <EventFieldModal
          field={picked.field}
          value={picked.value}
          investigationId={investigationId}
          eventInContext={eventInContext}
          onClose={() => setPicked(null)}
          onAddFilter={onAddFilter}
          onAddToContext={
            onAddToContext
              ? (includeEvent) => onAddToContext(picked.field, picked.value, includeEvent)
              : undefined
          }
        />
      )}
    </div>
  )
}
