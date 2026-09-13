import type { Position } from '../../shared/api/generated/types.gen';

/**
 * The local scheme of Bishkek the map draws instead of online tiles. It is a demonstration sketch
 * of the main streets and a few landmarks, drawn from coordinates held here, so the map works with
 * no tile provider, no external service and no key. It is not a survey and not a city plan.
 */
export const SCHEMATIC_CAPTION = 'Демонстрационная схема Бишкека · без онлайн-карт · Leaflet';

/** A street of the scheme. Avenues are drawn heavier than streets, as a map would draw them. */
export type SchematicStreet = { name: string; rank: 'avenue' | 'street'; path: Position[] };

/** A landmark of the scheme, drawn as a labelled point. */
export type SchematicLandmark = { name: string; at: Position };

export const SCHEMATIC_STREETS: readonly SchematicStreet[] = [
  {
    name: 'проспект Чуй',
    rank: 'avenue',
    path: [
      [74.552, 42.876],
      [74.648, 42.8755],
    ],
  },
  {
    name: 'улица Жибек Жолу',
    rank: 'avenue',
    path: [
      [74.552, 42.8885],
      [74.648, 42.8875],
    ],
  },
  {
    name: 'улица Киевская',
    rank: 'street',
    path: [
      [74.556, 42.8718],
      [74.641, 42.8712],
    ],
  },
  {
    name: 'улица Московская',
    rank: 'street',
    path: [
      [74.558, 42.8668],
      [74.639, 42.8662],
    ],
  },
  {
    name: 'улица Ахунбаева',
    rank: 'street',
    path: [
      [74.558, 42.8512],
      [74.643, 42.8506],
    ],
  },
  {
    name: 'Южная магистраль',
    rank: 'avenue',
    path: [
      [74.556, 42.8446],
      [74.645, 42.8442],
    ],
  },
  {
    name: 'проспект Мира',
    rank: 'avenue',
    path: [
      [74.5688, 42.8425],
      [74.5688, 42.8905],
    ],
  },
  {
    name: 'проспект Манаса',
    rank: 'avenue',
    path: [
      [74.5872, 42.8432],
      [74.5872, 42.8912],
    ],
  },
  {
    name: 'бульвар Эркиндик',
    rank: 'street',
    path: [
      [74.6046, 42.8492],
      [74.6046, 42.8798],
    ],
  },
  {
    name: 'улица Абдрахманова',
    rank: 'street',
    path: [
      [74.6128, 42.8438],
      [74.6128, 42.8918],
    ],
  },
  {
    name: 'улица Байтик Баатыра',
    rank: 'avenue',
    path: [
      [74.6302, 42.8436],
      [74.6302, 42.8908],
    ],
  },
];

export const SCHEMATIC_LANDMARKS: readonly SchematicLandmark[] = [
  { name: 'Площадь Ала-Тоо', at: [74.6035, 42.8762] },
  { name: 'Дубовый парк', at: [74.5998, 42.8785] },
  { name: 'Ошский рынок', at: [74.5722, 42.872] },
  { name: 'Аламединский рынок', at: [74.6358, 42.8758] },
  { name: 'Западный автовокзал', at: [74.5582, 42.8623] },
  { name: 'Железнодорожный вокзал', at: [74.6082, 42.8628] },
  { name: 'Парк Ата-Тюрк', at: [74.5902, 42.8482] },
];
