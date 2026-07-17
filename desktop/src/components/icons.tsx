// Minimal 16px stroke icons for the sidebar nav. Kept inline and monochrome so
// they inherit currentColor and stay on-theme.

type P = { className?: string }
const base = 'shrink-0'

function svg(children: React.ReactNode, className?: string) {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.4"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={`${base} ${className ?? ''}`}
    >
      {children}
    </svg>
  )
}

export const IconServer = ({ className }: P) =>
  svg(
    <>
      <rect x="2" y="2.5" width="12" height="4.5" rx="1" />
      <rect x="2" y="9" width="12" height="4.5" rx="1" />
      <path d="M4.5 4.75h.01M4.5 11.25h.01" />
    </>,
    className,
  )

export const IconPulse = ({ className }: P) =>
  svg(<path d="M1.5 8h3l2-4.5 3 9 2-4.5h3" />, className)

export const IconRows = ({ className }: P) =>
  svg(
    <>
      <path d="M2.5 4h11M2.5 8h11M2.5 12h11" />
    </>,
    className,
  )

export const IconGear = ({ className }: P) =>
  svg(
    <>
      <circle cx="8" cy="8" r="2.2" />
      <path d="M8 1.5v1.7M8 12.8v1.7M1.5 8h1.7M12.8 8h1.7M3.4 3.4l1.2 1.2M11.4 11.4l1.2 1.2M12.6 3.4l-1.2 1.2M4.6 11.4l-1.2 1.2" />
    </>,
    className,
  )

export const IconBox = ({ className }: P) =>
  svg(
    <>
      <path d="M8 1.8 14 5v6l-6 3.2L2 11V5z" />
      <path d="M2 5l6 3.2L14 5M8 8.2v6" />
    </>,
    className,
  )

export const IconTerminal = ({ className }: P) =>
  svg(
    <>
      <rect x="1.8" y="2.8" width="12.4" height="10.4" rx="1" />
      <path d="M4.5 6l2 2-2 2M8.5 10.5h3" />
    </>,
    className,
  )

export const IconPlus = ({ className }: P) => svg(<path d="M8 3v10M3 8h10" />, className)

export const IconGrid = ({ className }: P) =>
  svg(
    <>
      <rect x="2.2" y="2.2" width="4.6" height="4.6" rx="1" />
      <rect x="9.2" y="2.2" width="4.6" height="4.6" rx="1" />
      <rect x="2.2" y="9.2" width="4.6" height="4.6" rx="1" />
      <rect x="9.2" y="9.2" width="4.6" height="4.6" rx="1" />
    </>,
    className,
  )
