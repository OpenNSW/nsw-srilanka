import { TraderZoneLayout } from './components/TraderZoneLayout'
import { SAMPLE_COMPLETED_TASK, SAMPLE_TASK } from './fixtures'

export function ZonePreviewScreen() {
  return (
    <div className="min-h-screen bg-app-bg space-y-16 py-8">
      <TraderZoneLayout task={SAMPLE_TASK} />
      <TraderZoneLayout task={SAMPLE_COMPLETED_TASK} />
    </div>
  )
}
