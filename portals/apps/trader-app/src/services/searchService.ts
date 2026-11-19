import type { SearchServiceRegistry } from '@opennsw/jsonforms-renderers'
import { chaSearchService } from '@/features/cha/cha'
import { staticDataSearchService, withManifestVersion } from '@/features/static-data/service'

export const searchServices: SearchServiceRegistry = {
  cha: chaSearchService,
  'static-data': staticDataSearchService,
}

// taskSearchServices is searchServices for a task's forms: their static data loads as the
// task's manifest version lists it.
export function taskSearchServices(manifestVersion: number): SearchServiceRegistry {
  return { ...searchServices, 'static-data': withManifestVersion(staticDataSearchService, manifestVersion) }
}
