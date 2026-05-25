import { TraderZoneLayout } from '../zones/TraderZoneLayout'
import { SAMPLE_TASK } from '../zones/fixtures'

export function ZonePreviewScreen() {
  return (
    <div className="min-h-screen bg-gray-50">
      <TraderZoneLayout task={SAMPLE_TASK} />
    </div>
  )
}
