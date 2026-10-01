import { TraderZoneLayout } from './components/TraderZoneLayout'
import { SAMPLE_TASK } from './fixtures'

export function ZonePreviewScreen() {
  return (
    <div className="min-h-screen bg-app-bg pb-16 sm:pb-8">
      <TraderZoneLayout task={SAMPLE_TASK} />
    </div>
  )
}
