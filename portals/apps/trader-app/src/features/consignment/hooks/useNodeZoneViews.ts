import { useEffect, useMemo, useState } from 'react'
import type { WorkflowNode } from '@/features/consignment/types'
import { getZoneView } from '@/features/task/service'
import type { ZoneView } from '@/features/zone/types'
import { logger } from '@/utils/logger'

/**
 * Load projected zone views for active consignment nodes so the list can
 * overlay Pending Feedback from the same GET /tasks/{id} payload the task
 * page already uses.
 */
export function useNodeZoneViews(nodes: WorkflowNode[]): Record<string, ZoneView> {
  const [zones, setZones] = useState<Record<string, ZoneView>>({})
  const active = useMemo(
    () =>
      nodes
        .filter((n) => n.state === 'IN_PROGRESS' || n.state === 'READY')
        .map((n) => ({ id: n.id, updatedAt: n.updatedAt })),
    [nodes],
  )

  useEffect(() => {
    const ids = active.map((n) => n.id)
    if (ids.length === 0) {
      setZones({})
      return
    }

    let cancelled = false
    void Promise.all(
      ids.map(async (id) => {
        try {
          const zone = await getZoneView(id)
          return [id, zone] as const
        } catch (err) {
          logger.error('Failed to load task view for consignment node', id, err)
          return null
        }
      }),
    ).then((rows) => {
      if (cancelled) return
      const next: Record<string, ZoneView> = {}
      for (const row of rows) {
        if (row) {
          next[row[0]] = row[1]
        }
      }
      setZones(next)
    })

    return () => {
      cancelled = true
    }
  }, [active])

  return zones
}
