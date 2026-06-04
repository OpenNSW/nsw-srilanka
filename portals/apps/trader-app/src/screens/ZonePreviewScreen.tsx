import { TraderZoneLayout } from '../zones/TraderZoneLayout'
import { SAMPLE_TASK } from '../zones/fixtures'

export function ZonePreviewScreen() {
  return (
    <div className="min-h-screen bg-surface">
      <TraderZoneLayout task={SAMPLE_TASK} />
    </div>
  )
}
