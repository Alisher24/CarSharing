/** The label the card offers when a check may be started again. */
const RECHECK_ACTION = 'Проверить соединение';

/**
 * What the connection card says about each state it can be in. The copy is data rather than a
 * chain of conditions in the component, so every state the card can reach is answered in one place.
 */
export const CONNECTION_COPY = {
  loading: {
    title: 'Проверяем соединение…',
    description: 'Запрашиваем актуальные данные.',
    action: RECHECK_ACTION,
  },
  ready: {
    title: 'Сервис на связи',
    description: 'Соединение установлено. Выбор автомобиля и бронирование появятся в следующих версиях.',
    action: RECHECK_ACTION,
  },
  error: {
    title: 'Нет связи с сервисом',
    description: 'Не удалось получить ответ. Проверьте, запущен ли сервис, и повторите попытку.',
    action: 'Повторить попытку',
  },
} as const;
