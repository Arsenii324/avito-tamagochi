import type { PetMood, PresetId } from '@/shared/types';

// Ави рисуется здесь, а не картинкой из файла: SVG инлайном не грузится по сети
// (демо-стенд отдаётся с голого IP), масштабируется без артефактов и умеет
// менять выражение по настроению, чего статичный png не умеет.
//
// ВАЖНО: четыре круга Авито как персонажа рисовать нельзя (AGENTS.md → Never).
// Здесь берутся только ЦВЕТА логотипа и ровно в тех ролях, которые называет
// контракт (docs/openapi.json → PetPreset.colors: «Четыре цвета логотипа Авито
// в ролях: тело, большое ухо, малое ухо, лапа»). Форма — зверёк, не знак.
const AVITO = {
  green: '#04E061',
  blue: '#00AAFF',
  purple: '#965EEB',
  coral: '#FF4053',
} as const;

interface PresetColors {
  body: string;
  earL: string;
  earS: string;
  paw: string;
}

// Пресет решает, какой из цветов достаётся телу; остальные три расходятся по
// ушам и лапам, чтобы зверёк оставался четырёхцветным при любом выборе.
const PRESET_COLORS: Record<PresetId, PresetColors> = {
  green: { body: AVITO.green, earL: AVITO.purple, earS: AVITO.blue, paw: AVITO.coral },
  blue: { body: AVITO.blue, earL: AVITO.green, earS: AVITO.purple, paw: AVITO.coral },
  purple: { body: AVITO.purple, earL: AVITO.coral, earS: AVITO.green, paw: AVITO.blue },
};

// Выражение — единственное, что зависит от настроения. Глаза и рот заданы
// путями, а не эмодзи: эмодзи рендерятся по-разному в разных системах, а на
// защите картинка должна быть той же, что видели мы.
const FACES: Record<PetMood, { eyes: React.ReactNode; mouth: string }> = {
  radiant: {
    eyes: (
      <>
        <path
          d="M40 46 q6 -8 12 0"
          stroke="#1A1A1A"
          strokeWidth="3.5"
          fill="none"
          strokeLinecap="round"
        />
        <path
          d="M68 46 q6 -8 12 0"
          stroke="#1A1A1A"
          strokeWidth="3.5"
          fill="none"
          strokeLinecap="round"
        />
      </>
    ),
    mouth: 'M50 62 q10 12 20 0',
  },
  happy: {
    eyes: (
      <>
        <circle cx="46" cy="46" r="4.5" fill="#1A1A1A" />
        <circle cx="74" cy="46" r="4.5" fill="#1A1A1A" />
      </>
    ),
    mouth: 'M52 62 q8 9 16 0',
  },
  neutral: {
    eyes: (
      <>
        <circle cx="46" cy="46" r="4" fill="#1A1A1A" />
        <circle cx="74" cy="46" r="4" fill="#1A1A1A" />
      </>
    ),
    mouth: 'M53 64 h14',
  },
  sad: {
    eyes: (
      <>
        <circle cx="46" cy="48" r="4" fill="#1A1A1A" />
        <circle cx="74" cy="48" r="4" fill="#1A1A1A" />
      </>
    ),
    mouth: 'M52 68 q8 -9 16 0',
  },
  sick: {
    eyes: (
      <>
        <path d="M42 43 l8 8 M50 43 l-8 8" stroke="#1A1A1A" strokeWidth="3" strokeLinecap="round" />
        <path d="M70 43 l8 8 M78 43 l-8 8" stroke="#1A1A1A" strokeWidth="3" strokeLinecap="round" />
      </>
    ),
    mouth: 'M52 66 q4 -6 8 0 q4 6 8 0',
  },
  sleeping: {
    eyes: (
      <>
        <path
          d="M40 47 q6 5 12 0"
          stroke="#1A1A1A"
          strokeWidth="3.5"
          fill="none"
          strokeLinecap="round"
        />
        <path
          d="M68 47 q6 5 12 0"
          stroke="#1A1A1A"
          strokeWidth="3.5"
          fill="none"
          strokeLinecap="round"
        />
      </>
    ),
    mouth: 'M56 64 q4 4 8 0',
  },
};

interface PetAvatarProps {
  presetId: PresetId;
  mood: PetMood;
  /** Сторона квадрата в пикселях. */
  size?: number;
}

export function PetAvatar({ presetId, mood, size = 132 }: PetAvatarProps) {
  const colors = PRESET_COLORS[presetId];
  const face = FACES[mood];

  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 120 120"
      role="img"
      aria-label={`Питомец, настроение: ${mood}`}
    >
      {/* Большое и малое ухо — разного размера, как называет их контракт. */}
      <ellipse cx="36" cy="24" rx="14" ry="18" fill={colors.earL} />
      <ellipse cx="82" cy="28" rx="10" ry="13" fill={colors.earS} />

      {/* Тело. */}
      <ellipse cx="60" cy="58" rx="38" ry="36" fill={colors.body} />

      {/* Лапы. */}
      <ellipse cx="42" cy="96" rx="13" ry="9" fill={colors.paw} />
      <ellipse cx="78" cy="96" rx="13" ry="9" fill={colors.paw} />

      {/* Светлое пятно на животе — чтобы силуэт не читался плоским. */}
      <ellipse cx="60" cy="70" rx="20" ry="16" fill="#FFFFFF" opacity="0.22" />

      {face.eyes}
      <path d={face.mouth} stroke="#1A1A1A" strokeWidth="3.5" fill="none" strokeLinecap="round" />

      {/* Спящему — «z», сияющему — искры. Больше ни у кого декора нет. */}
      {mood === 'sleeping' && (
        <text x="96" y="34" fontSize="18" fontWeight="700" fill="#1A1A1A" opacity="0.55">
          z
        </text>
      )}
      {mood === 'radiant' && (
        <>
          <path d="M14 40 l3 -7 3 7 7 3 -7 3 -3 7 -3 -7 -7 -3 z" fill={AVITO.coral} />
          <path d="M100 62 l2 -5 2 5 5 2 -5 2 -2 5 -2 -5 -5 -2 z" fill={AVITO.green} />
        </>
      )}
    </svg>
  );
}
