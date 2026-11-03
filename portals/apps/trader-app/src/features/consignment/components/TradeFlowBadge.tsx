import { useTranslation } from 'react-i18next'
import { StatusBadge } from '@portals/ui'
import type { TradeFlow } from '@/features/consignment/types'

// Whether a consignment is an import or an export.
export function TradeFlowBadge({ flow }: { flow: TradeFlow }) {
  const { t } = useTranslation()
  return (
    <StatusBadge tone={flow === 'IMPORT' ? 'info' : 'success'}>
      {flow === 'IMPORT' ? t('consignments.tradeFlow.import') : t('consignments.tradeFlow.export')}
    </StatusBadge>
  )
}
