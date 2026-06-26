import { TraderZoneLayout } from './components/TraderZoneLayout'
import { SAMPLE_TASK } from './fixtures'

export function ZonePreviewScreen() {
  return (
    <div className="min-h-screen bg-surface">
      <TraderZoneLayout task={SAMPLE_TASK} />
    </div>
  )
}
