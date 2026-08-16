import type { ReactNode, SVGProps } from "react";

// Единый набор тонких контурных иконок (stroke, currentColor, 24×24) — общий стиль для всего проекта.
type IconProps = SVGProps<SVGSVGElement> & { size?: number };

function Icon({ size = 20, children, ...props }: IconProps & { children: ReactNode }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.7}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
      {...props}
    >
      {children}
    </svg>
  );
}

export const IconSearch = (p: IconProps) => (
  <Icon {...p}>
    <circle cx="11" cy="11" r="7" />
    <path d="m20 20-3.2-3.2" />
  </Icon>
);

export const IconMessage = (p: IconProps) => (
  <Icon {...p}>
    <path d="M21 11.5a8.4 8.4 0 0 1-8.5 8.4 8.7 8.7 0 0 1-3.8-.9L3 20.5l1.6-4.6a8.4 8.4 0 0 1-.9-3.9A8.4 8.4 0 0 1 12.2 3 8.4 8.4 0 0 1 21 11.5Z" />
  </Icon>
);

export const IconBell = (p: IconProps) => (
  <Icon {...p}>
    <path d="M6 9a6 6 0 0 1 12 0c0 5 2.2 6.5 2.2 6.5H3.8S6 14 6 9Z" />
    <path d="M10 20a2 2 0 0 0 4 0" />
  </Icon>
);

export const IconUser = (p: IconProps) => (
  <Icon {...p}>
    <circle cx="12" cy="8" r="4" />
    <path d="M4 20a8 8 0 0 1 16 0" />
  </Icon>
);

export const IconBriefcase = (p: IconProps) => (
  <Icon {...p}>
    <rect x="3" y="7" width="18" height="13" rx="2" />
    <path d="M8 7V5a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2M3 12h18" />
  </Icon>
);

export const IconStar = (p: IconProps) => (
  <Icon {...p}>
    <path d="M12 3.5 14.6 9l6 .5-4.5 3.9 1.4 5.8L12 16.9 6.5 19.2l1.4-5.8L3.4 9.5l6-.5L12 3.5Z" />
  </Icon>
);

export const IconArrowRight = (p: IconProps) => (
  <Icon {...p}>
    <path d="M5 12h14M13 6l6 6-6 6" />
  </Icon>
);

export const IconClock = (p: IconProps) => <Icon {...p}><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></Icon>;
export const IconWallet = (p: IconProps) => <Icon {...p}><path d="M4 7h14a2 2 0 0 1 2 2v9H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h11v3"/><path d="M15 12h5"/></Icon>;
export const IconShield = (p: IconProps) => <Icon {...p}><path d="M12 3 20 6v5c0 5-3.2 8.1-8 10-4.8-1.9-8-5-8-10V6l8-3Z"/><path d="m9 12 2 2 4-4"/></Icon>;
export const IconBook = (p: IconProps) => <Icon {...p}><path d="M4 5.5A2.5 2.5 0 0 1 6.5 3H11v16H6.5A2.5 2.5 0 0 0 4 21.5v-16Z"/><path d="M20 5.5A2.5 2.5 0 0 0 17.5 3H13v16h4.5A2.5 2.5 0 0 1 20 21.5v-16Z"/></Icon>;
export const IconHeart = (p: IconProps) => <Icon {...p}><path d="M20.8 4.6a5.5 5.5 0 0 0-7.8 0L12 5.7l-1.1-1.1a5.5 5.5 0 0 0-7.8 7.8L12 21l8.8-8.6a5.5 5.5 0 0 0 0-7.8Z"/></Icon>;
export const IconTag = (p: IconProps) => <Icon {...p}><path d="M20 13 13 20 4 11V4h7l9 9Z"/><circle cx="8.5" cy="8.5" r="1.2"/></Icon>;
export const IconMapPin = (p: IconProps) => <Icon {...p}><path d="M20 10c0 5-8 11-8 11S4 15 4 10a8 8 0 1 1 16 0Z"/><circle cx="12" cy="10" r="2.5"/></Icon>;
export const IconImage = (p: IconProps) => <Icon {...p}><rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="8.5" cy="9" r="1.5"/><path d="m21 15-5-5L5 20"/></Icon>;
export const IconFilter = (p: IconProps) => <Icon {...p}><path d="M4 6h16M7 12h10M10 18h4"/></Icon>;
export const IconCheck = (p: IconProps) => <Icon {...p}><path d="m5 12 4 4L19 6"/></Icon>;
export const IconSparkles = (p: IconProps) => <Icon {...p}><path d="m12 3 1.2 3.2L16 8l-2.8 1.8L12 13l-1.2-3.2L8 8l2.8-1.8L12 3ZM5 14l.8 2.1L8 17.5l-2.2 1.4L5 21l-.8-2.1L2 17.5l2.2-1.4L5 14ZM19 12l.7 1.8 1.8.7-1.8.7L19 17l-.7-1.8-1.8-.7 1.8-.7L19 12Z"/></Icon>;

export const IconCode = (p: IconProps) => <Icon {...p}><path d="m8 9-4 3 4 3"/><path d="m16 9 4 3-4 3"/><path d="m14 5-4 14"/></Icon>;
export const IconPalette = (p: IconProps) => <Icon {...p}><path d="M12 3a9 9 0 0 0 0 18h1.2a2.1 2.1 0 0 0 1.5-3.6 1.8 1.8 0 0 1 1.2-3.1H18A3 3 0 0 0 21 11a8 8 0 0 0-9-8Z"/><circle cx="7.5" cy="10" r=".7"/><circle cx="10" cy="6.8" r=".7"/><circle cx="14" cy="6.5" r=".7"/><circle cx="17" cy="9" r=".7"/></Icon>;
export const IconMegaphone = (p: IconProps) => <Icon {...p}><path d="M4 13V9l12-4v12L4 13Z"/><path d="M16 9a4 4 0 0 1 0 4"/><path d="m7 14 1.5 5h3L10 13"/></Icon>;
export const IconChart = (p: IconProps) => <Icon {...p}><path d="M4 20V10"/><path d="M10 20V4"/><path d="M16 20v-7"/><path d="M22 20H2"/></Icon>;
export const IconPenTool = (p: IconProps) => <Icon {...p}><path d="m12 19 7-7-7-7-7 7 7 7Z"/><path d="M12 5v6"/><circle cx="12" cy="13" r="2"/></Icon>;
export const IconDatabase = (p: IconProps) => <Icon {...p}><ellipse cx="12" cy="5" rx="8" ry="3"/><path d="M4 5v6c0 1.7 3.6 3 8 3s8-1.3 8-3V5"/><path d="M4 11v6c0 1.7 3.6 3 8 3s8-1.3 8-3v-6"/></Icon>;

export const IconGrid = (p: IconProps) => <Icon {...p}><rect x="4" y="4" width="6" height="6" rx="1"/><rect x="14" y="4" width="6" height="6" rx="1"/><rect x="4" y="14" width="6" height="6" rx="1"/><rect x="14" y="14" width="6" height="6" rx="1"/></Icon>;
export const IconList = (p: IconProps) => <Icon {...p}><path d="M8 6h12M8 12h12M8 18h12"/><circle cx="4" cy="6" r="1"/><circle cx="4" cy="12" r="1"/><circle cx="4" cy="18" r="1"/></Icon>;
export const IconChevronDown = (p: IconProps) => <Icon {...p}><path d="m7 10 5 5 5-5"/></Icon>;
export const IconMenu = (p: IconProps) => <Icon {...p}><path d="M4 6h16M4 12h16M4 18h16"/></Icon>;
export const IconX = (p: IconProps) => <Icon {...p}><path d="m6 6 12 12M18 6 6 18"/></Icon>;
export const IconRefresh = (p: IconProps) => <Icon {...p}><path d="M20 6v5h-5"/><path d="M4 18v-5h5"/><path d="M6.1 8A7 7 0 0 1 18 6l2 5M4 13l2 5a7 7 0 0 0 11.9-2"/></Icon>;
export const IconInbox = (p: IconProps) => <Icon {...p}><path d="M4 4h16v12a4 4 0 0 1-4 4H8a4 4 0 0 1-4-4V4Z"/><path d="M4 13h5l2 3h2l2-3h5"/></Icon>;
export const IconMic = (p: IconProps) => <Icon {...p}><rect x="9" y="3" width="6" height="11" rx="3"/><path d="M5 11a7 7 0 0 0 14 0M12 18v3M9 21h6"/></Icon>;
export const IconUserCircle = (p: IconProps) => <Icon {...p}><circle cx="12" cy="12" r="9"/><circle cx="12" cy="9" r="3"/><path d="M6.8 18a6 6 0 0 1 10.4 0"/></Icon>;

export const IconCalendar = (p: IconProps) => <Icon {...p}><rect x="3" y="5" width="18" height="16" rx="3"/><path d="M8 3v4M16 3v4M3 10h18"/><path d="M8 14h.01M12 14h.01M16 14h.01M8 18h.01M12 18h.01"/></Icon>;
