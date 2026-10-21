import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";

// Ikki tilli UI (uz / ru). Kutubxonasiz: lug'at + context + `t()`.
// Tanlangan til localStorage("lang") da saqlanadi; default — uz.
// Yangi matn qo'shganda ikkala lug'atga ham kiriting (TypeScript buni majburlaydi).

export type Lang = "uz" | "ru";

const uz = {
  // Layout / nav
  navLogViewer: "Log Viewer",
  navJobs: "Jobs",
  navSettings: "Settings",
  navUsers: "Users",
  themeLight: "Light mode",
  themeDark: "Dark mode",
  logout: "Chiqish",
  langSwitchTitle: "Tilni o'zgartirish",

  // Login
  loginSubtitle: "Tizimga kirish",
  loginError: "Login yoki parol noto'g'ri",
  username: "Username",
  password: "Password",
  loginButton: "Kirish",

  // Common
  save: "Saqlash",
  cancel: "Bekor qilish",
  cancelShort: "Bekor",
  delete: "O'chirish",
  edit: "Tahrirlash",
  close: "Yopish",
  loading: "Yuklanmoqda...",
  genericError: "Xatolik yuz berdi",
  saved: "Saqlandi!",
  unlimited: "Cheksiz",
  name: "Nomi",
  globalBadge: "Global",
  globalHint: "Global (barcha userlarga ko'rinadi)",
  datasource: "Datasource",

  // Log Viewer
  savedQueries: "Saved Queries",
  noQueries: "Querylar yo'q",
  lvTitle: "Log Viewer",
  lvSubtitle: "LogsQL query yozing va loglarni ko'ring",
  from: "From",
  to: "To",
  fromPlaceholder: "Hammasi (cheksiz)",
  toPlaceholder: "Hammasi (hozirgacha)",
  presetAll: "Hammasi",
  presetAllTitle: "Vaqt cheklovini olib tashlash (barcha loglar)",
  tzHint: "Mahalliy vaqt ({tz}) — server'ga UTC'da yuboriladi. Jadvaldagi _time ham mahalliyga aylantirilgan (asl UTC qiymat tooltip'da).",
  noDatasourceHint: "Settings dan datasource qo'shing",
  queryPlaceholder: 'Masalan: level:"error" AND service:"api-gateway"',
  search: "Search",
  saveQuery: "Save Query",
  fileNamePlaceholder: "Fayl nomi (ixtiyoriy)",
  exportFormatLocked: "Format: Avtomatik (.tar.gz)",
  exportFormatLockedTitle: "Export doim .tar.gz formatida yaratiladi",
  export: "Export",
  editQueryTitle: "Queryni tahrirlash",
  queryNamePlaceholder: "Query nomi",
  query: "Query",
  dsNotSelected: "— tanlanmagan —",
  queryError: "So'rov xatosi",
  saveQueryTitle: "Query saqlash",
  logsFound: "{n} ta log topildi",
  page: "Sahifa {n}",
  noLogs: "Hech qanday log topilmadi",
  prev: "Oldingi",
  next: "Keyingi",
  sortDescTitle: "_time bo'yicha kamayish tartibida saralash",
  sortAscTitle: "_time bo'yicha o'sish tartibida saralash",

  // Jobs
  jobsTitle: "Jobs",
  jobsTotal: "Jami {n} ta job",
  runningCount: "{n} running",
  doneCount: "{n} done",
  failedCount: "{n} failed",
  sectionRunning: "Running",
  sectionDone: "Done",
  sectionFailed: "Failed",
  statusRunning: "RUNNING",
  statusDone: "DONE",
  statusFailed: "FAILED",
  noJobs: "Hozircha hech qanday job yo'q",
  linesCount: "{n} qator",
  owner: "Egasi",
  share: "Share",
  shareButtonTitle: "Share link yaratish / boshqarish",
  download: "Yuklab olish",
  fileNameTitle: "Yuklab olinadigan fayl nomi",
  jobFrom: "From:",
  jobTo: "To:",
  shareTitle: "Share link",
  link: "Link",
  copy: "Copy",
  copied: "Ko'chirildi!",
  downloadsStat: "Yuklab olishlar",
  expiryStat: "Muddati",
  shareWarning: "Limit yetganda yoki muddat o'tganda fayl o'chiriladi.",
  shareWarningAuto: "Limit yetganda yoki muddat o'tganda fayl avtomatik o'chiriladi.",
  revoke: "Bekor qilish (Revoke)",
  maxDownloads: "Maksimal yuklab olishlar",
  create: "Yaratish",
  shareError: "Share yaratishda xatolik",
  clipboardError: "Clipboard'ga ko'chirib bo'lmadi",
  hours1: "1 soat",
  hours6: "6 soat",
  hours24: "24 soat",
  days7: "7 kun",

  // Settings
  settingsTitle: "Settings",
  settingsSubtitle: "VictoriaLogs datasource'larni boshqarish",
  url: "URL",
  basicAuth: "Basic Auth (ixtiyoriy)",
  passwordSavedPlaceholder: "Saqlangan (o'zgartirish uchun yangisini yozing)",
  headers: "Headers (ixtiyoriy)",
  headerNamePlaceholder: "Header name (masalan: X-Tenant)",
  headerValue: "Value",
  removeHeader: "Header o'chirish",
  addHeader: "+ Header qo'shish",
  unnamed: "Nomsiz",
  addDatasource: "+ Datasource qo'shish",

  // Users
  usersTitle: "Users",
  usersSubtitle: "Foydalanuvchilarni boshqarish",
  roleUser: "User",
  roleAdmin: "Admin",
  addUser: "Qo'shish",
  editUserTitle: "Userni tahrirlash",
  role: "Role",
  newPasswordLabel: "Yangi parol (bo'sh qoldirilsa o'zgarmaydi)",
  newPasswordPlaceholder: "Yangi parol",
  saveError: "Saqlashda xatolik",
};

export type TKey = keyof typeof uz;

const ru: Record<TKey, string> = {
  navLogViewer: "Просмотр логов",
  navJobs: "Задачи",
  navSettings: "Настройки",
  navUsers: "Пользователи",
  themeLight: "Светлая тема",
  themeDark: "Тёмная тема",
  logout: "Выйти",
  langSwitchTitle: "Сменить язык",

  loginSubtitle: "Вход в систему",
  loginError: "Неверный логин или пароль",
  username: "Логин",
  password: "Пароль",
  loginButton: "Войти",

  save: "Сохранить",
  cancel: "Отмена",
  cancelShort: "Отмена",
  delete: "Удалить",
  edit: "Редактировать",
  close: "Закрыть",
  loading: "Загрузка...",
  genericError: "Произошла ошибка",
  saved: "Сохранено!",
  unlimited: "Без ограничений",
  name: "Название",
  globalBadge: "Общий",
  globalHint: "Общий (виден всем пользователям)",
  datasource: "Источник данных",

  savedQueries: "Сохранённые запросы",
  noQueries: "Нет запросов",
  lvTitle: "Просмотр логов",
  lvSubtitle: "Введите LogsQL-запрос и просмотрите логи",
  from: "С",
  to: "По",
  fromPlaceholder: "Все (без ограничения)",
  toPlaceholder: "Все (до текущего момента)",
  presetAll: "Все",
  presetAllTitle: "Снять ограничение по времени (все логи)",
  tzHint: "Локальное время ({tz}) — на сервер отправляется в UTC. _time в таблице тоже переведён в локальное (исходное UTC — в подсказке).",
  noDatasourceHint: "Добавьте источник данных в Настройках",
  queryPlaceholder: 'Например: level:"error" AND service:"api-gateway"',
  search: "Поиск",
  saveQuery: "Сохранить запрос",
  fileNamePlaceholder: "Имя файла (необязательно)",
  exportFormatLocked: "Формат: Автоматически (.tar.gz)",
  exportFormatLockedTitle: "Экспорт всегда создаётся в формате .tar.gz",
  export: "Экспорт",
  editQueryTitle: "Редактировать запрос",
  queryNamePlaceholder: "Название запроса",
  query: "Запрос",
  dsNotSelected: "— не выбран —",
  queryError: "Ошибка запроса",
  saveQueryTitle: "Сохранить запрос",
  logsFound: "Найдено логов: {n}",
  page: "Страница {n}",
  noLogs: "Логи не найдены",
  prev: "Назад",
  next: "Вперёд",
  sortDescTitle: "Сортировать по _time по убыванию",
  sortAscTitle: "Сортировать по _time по возрастанию",

  jobsTitle: "Задачи",
  jobsTotal: "Всего задач: {n}",
  runningCount: "{n} в работе",
  doneCount: "{n} готово",
  failedCount: "{n} с ошибкой",
  sectionRunning: "Выполняются",
  sectionDone: "Готовые",
  sectionFailed: "С ошибкой",
  statusRunning: "В РАБОТЕ",
  statusDone: "ГОТОВО",
  statusFailed: "ОШИБКА",
  noJobs: "Пока нет ни одной задачи",
  linesCount: "{n} строк",
  owner: "Владелец",
  share: "Поделиться",
  shareButtonTitle: "Создать / управлять ссылкой",
  download: "Скачать",
  fileNameTitle: "Имя скачиваемого файла",
  jobFrom: "С:",
  jobTo: "По:",
  shareTitle: "Ссылка для скачивания",
  link: "Ссылка",
  copy: "Копировать",
  copied: "Скопировано!",
  downloadsStat: "Скачиваний",
  expiryStat: "Срок действия",
  shareWarning: "Файл будет удалён при достижении лимита или истечении срока.",
  shareWarningAuto: "Файл будет автоматически удалён при достижении лимита или истечении срока.",
  revoke: "Отозвать",
  maxDownloads: "Максимум скачиваний",
  create: "Создать",
  shareError: "Ошибка создания ссылки",
  clipboardError: "Не удалось скопировать в буфер обмена",
  hours1: "1 час",
  hours6: "6 часов",
  hours24: "24 часа",
  days7: "7 дней",

  settingsTitle: "Настройки",
  settingsSubtitle: "Управление источниками данных VictoriaLogs",
  url: "URL",
  basicAuth: "Basic Auth (необязательно)",
  passwordSavedPlaceholder: "Сохранён (введите новый, чтобы изменить)",
  headers: "Заголовки (необязательно)",
  headerNamePlaceholder: "Имя заголовка (например: X-Tenant)",
  headerValue: "Значение",
  removeHeader: "Удалить заголовок",
  addHeader: "+ Добавить заголовок",
  unnamed: "Без названия",
  addDatasource: "+ Добавить источник",

  usersTitle: "Пользователи",
  usersSubtitle: "Управление пользователями",
  roleUser: "Пользователь",
  roleAdmin: "Администратор",
  addUser: "Добавить",
  editUserTitle: "Редактировать пользователя",
  role: "Роль",
  newPasswordLabel: "Новый пароль (пусто — без изменений)",
  newPasswordPlaceholder: "Новый пароль",
  saveError: "Ошибка сохранения",
};

const dicts: Record<Lang, Record<TKey, string>> = { uz, ru };

export type TVars = Record<string, string | number>;

function interpolate(template: string, vars?: TVars): string {
  if (!vars) return template;
  return template.replace(/\{(\w+)\}/g, (m, k: string) => (k in vars ? String(vars[k]) : m));
}

export function getInitialLang(): Lang {
  try {
    const stored = localStorage.getItem("lang");
    if (stored === "uz" || stored === "ru") return stored;
  } catch {
    /* private mode / blocked storage */
  }
  return "uz";
}

interface LangContextValue {
  lang: Lang;
  setLang: (l: Lang) => void;
  t: (key: TKey, vars?: TVars) => string;
}

const LangContext = createContext<LangContextValue>({
  lang: "uz",
  setLang: () => {},
  t: (key, vars) => interpolate(uz[key], vars),
});

export function LangProvider({ children }: { children: ReactNode }) {
  const [lang, setLang] = useState<Lang>(getInitialLang);

  useEffect(() => {
    try {
      localStorage.setItem("lang", lang);
    } catch {
      /* ignore */
    }
    document.documentElement.lang = lang;
  }, [lang]);

  const t = useCallback(
    (key: TKey, vars?: TVars) => interpolate(dicts[lang][key] ?? uz[key] ?? key, vars),
    [lang],
  );

  const value = useMemo(() => ({ lang, setLang, t }), [lang, t]);
  return <LangContext.Provider value={value}>{children}</LangContext.Provider>;
}

export function useT(): LangContextValue {
  return useContext(LangContext);
}
