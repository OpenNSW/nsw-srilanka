import { z } from 'zod'

export const UIConfigSchema = z.object({
  branding: z.object({
    systemName: z.string().min(1),
    appName: z.string().min(1),
    logoUrl: z.string().optional(),
    systemLogoUrl: z.string().optional(),
    favicon: z.string().optional(),
    portalName: z.string().optional(),
    description: z.string().optional(),
    heroImageUrl: z.string().optional(),
    partnerLogos: z.array(z.object({ url: z.string(), alt: z.string() })).optional(),
    // key selects a known footer page — its visible label is translated in
    // this app's own i18n bundle by key (see Footer.tsx), not carried here,
    // so it renders correctly regardless of language. url is always an
    // absolute URL: this app has no policy/accessibility/support pages of
    // its own, these link out to wherever that content is actually hosted.
    footerLinks: z
      .array(
        z.object({
          key: z.enum(['policy', 'accessibility', 'support']),
          url: z
            .string()
            .url()
            .refine((value) => /^https?:\/\//.test(value), 'must be an absolute http(s) URL'),
        }),
      )
      .optional(),
    // Optional. Shown centered in the footer (bottom-most on narrow
    // screens). Free text — this app doesn't assume a country or legal
    // entity, so the exact wording is a per-deployment choice.
    copyrightNotice: z.string().optional(),
  }),
  theme: z
    .object({
      fontFamily: z.string(),
      borderRadius: z.string(),
    })
    .optional(),
  features: z
    .object({
      preConsignment: z.boolean(),
      consignmentManagement: z.boolean(),
      reportingDashboard: z.boolean(),
    })
    .optional(),
})

export type UIConfig = z.infer<typeof UIConfigSchema>

export function validateConfig(parsed: unknown, instanceId: string): UIConfig {
  const result = UIConfigSchema.safeParse(parsed)
  if (!result.success) {
    throw new Error(
      'Invalid configuration for ' +
        instanceId +
        ':\n' +
        result.error.issues.map((i) => '- ' + i.path.join('.') + ': ' + i.message).join('\n'),
    )
  }
  return result.data
}

export function getDisplayName(config: UIConfig): string {
  return config.branding.portalName || config.branding.appName
}
