import { Link } from 'react-router-dom'
import { ChevronRightIcon } from '@radix-ui/react-icons'
import { useTranslation } from 'react-i18next'

export interface BreadcrumbItem {
  label: string
  // Omitted on the current page, the last item.
  to?: string
}

// Where the user is, and the way back to each level above it:
// Consignments › <consignment> › <task>.
export function Breadcrumb({ items }: { items: BreadcrumbItem[] }) {
  const { t } = useTranslation()
  const link = 'text-foreground-muted hover:text-foreground hover:underline'
  return (
    <nav aria-label={t('common.breadcrumb')} className="min-w-0">
      <ol className="flex flex-wrap items-center gap-1.5 text-sm">
        {items.map((item, index) => (
          <li key={index} className="flex min-w-0 items-center gap-1.5">
            {index > 0 && <ChevronRightIcon aria-hidden className="shrink-0 text-foreground-muted" />}
            {item.to ? (
              <Link to={item.to} className={`${link} max-w-64 truncate`}>
                {item.label}
              </Link>
            ) : (
              <span aria-current="page" className="truncate font-medium text-foreground">
                {item.label}
              </span>
            )}
          </li>
        ))}
      </ol>
    </nav>
  )
}
