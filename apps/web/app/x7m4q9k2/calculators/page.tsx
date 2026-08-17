"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { CustomSelect } from "../../custom-select";
import { useToast } from "../../toast";
import { useAutoSlug } from "../../slug";
import {
  AdminCalculatorsSkeleton,
  AdminError,
  AdminHeader,
  adminRequest,
  formatMoney,
} from "../admin-ui";

type Option = { value: string; label: string };
type Question = {
  key: string;
  label: string;
  type: "SELECT" | "NUMBER" | "BOOLEAN";
  required: boolean;
  min?: number;
  max?: number;
  options?: Option[];
};
type Pricing = {
  baseline_min_kopecks: number;
  baseline_max_kopecks: number;
  duration_min_days: number;
  duration_max_days: number;
  option_basis_points: Record<string, Record<string, number>>;
  number_basis_points: Record<string, number>;
  boolean_basis_points: Record<string, number>;
  category_slug: string;
  skill_slugs: string[];
  assumptions: string[];
};
type CalculatorDefinition = {
  slug: string;
  title: string;
  intro: string;
  version: number;
  enabled: boolean;
  questions: Question[];
  pricing: Pricing;
};

const emptyPricing = (): Pricing => ({
  baseline_min_kopecks: 3000000,
  baseline_max_kopecks: 6000000,
  duration_min_days: 5,
  duration_max_days: 15,
  option_basis_points: {},
  number_basis_points: {},
  boolean_basis_points: {},
  category_slug: "",
  skill_slugs: [],
  assumptions: [],
});

const emptyCalculator = (): CalculatorDefinition => ({
  slug: "",
  title: "",
  intro: "",
  version: 0,
  enabled: false,
  questions: [],
  pricing: emptyPricing(),
});

const keyFor = (index: number) => `factor_${index + 1}`;

function CalculatorEditor({
  initial,
  creating,
  onSaved,
  onCancel,
}: {
  initial: CalculatorDefinition;
  creating?: boolean;
  onSaved: (item: CalculatorDefinition) => void;
  onCancel?: () => void;
}) {
  const { push } = useToast();
  const [value, setValue] = useState(initial);
  const [saving, setSaving] = useState(false);

  const set = <K extends keyof CalculatorDefinition>(
    key: K,
    next: CalculatorDefinition[K],
  ) => setValue((current) => ({ ...current, [key]: next }));

  const setPricing = <K extends keyof Pricing>(key: K, next: Pricing[K]) =>
    setValue((current) => ({
      ...current,
      pricing: { ...current.pricing, [key]: next },
    }));

  const { handleSlugInput } = useAutoSlug({
    initialSlug: initial.slug,
    title: value.title,
    onSlugChange: (slug) => set("slug", slug),
  });

  function addQuestion() {
    const key = keyFor(value.questions.length);
    setValue((current) => ({
      ...current,
      questions: [
        ...current.questions,
        { key, label: "Новый параметр", type: "BOOLEAN", required: true },
      ],
      pricing: {
        ...current.pricing,
        boolean_basis_points: {
          ...current.pricing.boolean_basis_points,
          [key]: 1000,
        },
      },
    }));
  }

  function removeQuestion(index: number) {
    setValue((current) => {
      const question = current.questions[index];
      const questions = current.questions.filter((_, i) => i !== index);
      const option = { ...current.pricing.option_basis_points };
      const number = { ...current.pricing.number_basis_points };
      const bool = { ...current.pricing.boolean_basis_points };
      delete option[question.key];
      delete number[question.key];
      delete bool[question.key];
      return {
        ...current,
        questions,
        pricing: {
          ...current.pricing,
          option_basis_points: option,
          number_basis_points: number,
          boolean_basis_points: bool,
        },
      };
    });
  }

  function updateQuestion(index: number, next: Question) {
    setValue((current) => {
      const old = current.questions[index];
      const questions = current.questions.map((question, i) =>
        i === index ? next : question,
      );
      const option = { ...current.pricing.option_basis_points };
      const number = { ...current.pricing.number_basis_points };
      const bool = { ...current.pricing.boolean_basis_points };

      if (old.key !== next.key) {
        option[next.key] = option[old.key] || {};
        number[next.key] = number[old.key] || 0;
        bool[next.key] = bool[old.key] || 0;
        delete option[old.key];
        delete number[old.key];
        delete bool[old.key];
      }

      if (next.type === "SELECT") {
        next.options = next.options?.length
          ? next.options
          : [
              { value: "base", label: "Базовый вариант" },
              { value: "advanced", label: "Расширенный вариант" },
            ];
        option[next.key] = Object.fromEntries(
          next.options.map((item, i) => [
            item.value,
            option[next.key]?.[item.value] ?? (i ? 2000 : 0),
          ]),
        );
        delete number[next.key];
        delete bool[next.key];
      } else if (next.type === "NUMBER") {
        next.options = [];
        number[next.key] = number[next.key] ?? 100;
        delete option[next.key];
        delete bool[next.key];
      } else {
        next.options = [];
        bool[next.key] = bool[next.key] ?? 1000;
        delete option[next.key];
        delete number[next.key];
      }

      return {
        ...current,
        questions,
        pricing: {
          ...current.pricing,
          option_basis_points: option,
          number_basis_points: number,
          boolean_basis_points: bool,
        },
      };
    });
  }

  function optionChange(
    questionIndex: number,
    optionIndex: number,
    next: Option,
    percent: number,
  ) {
    setValue((current) => {
      const question = current.questions[questionIndex];
      const previous = question.options?.[optionIndex];
      const options = (question.options ?? []).map((item, index) =>
        index === optionIndex ? next : item,
      );
      const questions = current.questions.map((item, index) =>
        index === questionIndex ? { ...item, options } : item,
      );
      const adjustments = {
        ...(current.pricing.option_basis_points[question.key] ?? {}),
      };
      if (previous && previous.value !== next.value)
        delete adjustments[previous.value];
      adjustments[next.value] = Math.round(percent * 100);
      return {
        ...current,
        questions,
        pricing: {
          ...current.pricing,
          option_basis_points: {
            ...current.pricing.option_basis_points,
            [question.key]: adjustments,
          },
        },
      };
    });
  }

  async function save(event: FormEvent) {
    event.preventDefault();
    if (!value.questions.length) {
      push({ kind: "error", title: "Добавьте хотя бы один вопрос" });
      return;
    }
    setSaving(true);
    try {
      const payload = {
        slug: value.slug.trim(),
        title: value.title.trim(),
        intro: value.intro.trim(),
        enabled: value.enabled,
        questions: value.questions,
        pricing: value.pricing,
        reason: creating
          ? "Создание нового калькулятора"
          : "Новая версия калькулятора",
      };
      const body = await adminRequest<{ data: CalculatorDefinition }>(
        creating
          ? "/api/v1/admin/calculators"
          : `/api/v1/admin/calculators/${value.slug}`,
        {
          method: creating ? "POST" : "PATCH",
          body: JSON.stringify(payload),
        },
      );
      onSaved(body.data);
      setValue(body.data);
      push({
        kind: "success",
        title: creating ? "Калькулятор создан" : "Новая версия сохранена",
        message:
          "Публичный расчёт использует базовый диапазон и процентные поправки выбранных параметров.",
      });
    } catch (reason) {
      push({
        kind: "error",
        title: "Не удалось сохранить калькулятор",
        message:
          reason instanceof Error
            ? reason.message
            : "Проверьте формулу и поля.",
      });
    } finally {
      setSaving(false);
    }
  }

  return (
    <form className="calculator-builder" onSubmit={save}>
      <header className="calculator-builder__header">
        <div>
          <span className="status-pill">
            {creating ? "Новый калькулятор" : `Версия ${value.version}`}
          </span>
          <h2>{value.title || "Без названия"}</h2>
          {!creating ? (
            <small>
              {formatMoney(value.pricing.baseline_min_kopecks)}–
              {formatMoney(value.pricing.baseline_max_kopecks)}
            </small>
          ) : null}
        </div>
        <label className="calculator-switch">
          <input
            type="checkbox"
            checked={value.enabled}
            onChange={(event) => set("enabled", event.target.checked)}
          />
          <span>Показывать на сайте</span>
        </label>
      </header>
      <section className="calculator-builder__section">
        <h3>Раздел</h3>
        <div className="field-row">
          <label>
            Адрес раздела (slug)
            <input
              required
              pattern="[a-z0-9]+(?:-[a-z0-9]+)*"
              maxLength={160}
              disabled={!creating}
              placeholder="avto-slug-ili-vvedite-vruchnuyu"
              value={value.slug}
              onChange={(event) => handleSlugInput(event.target.value)}
            />
          </label>
          <label>
            Название
            <input
              required
              maxLength={220}
              placeholder="Например: Разработка мобильного приложения"
              value={value.title}
              onChange={(event) => set("title", event.target.value)}
            />
          </label>
        </div>
        <label>
          Описание
          <textarea
            required
            maxLength={600}
            rows={2}
            placeholder="Краткое описание назначения калькулятора"
            value={value.intro}
            onChange={(event) => set("intro", event.target.value)}
          />
        </label>
      </section>
      <section className="calculator-builder__section">
        <h3>Базовый расчёт</h3>
        <p className="form-hint">
          Базовая вилка — типичная цена задачи без усложнений. Каждый ответ
          ниже добавляет к ней процент. Например, 20% увеличит обе границы вилки
          на 20%.
        </p>
        <div className="field-row">
          <label>
            Базовая цена от, ₽
            <input
              required
              type="number"
              min={1}
              step={1000}
              value={value.pricing.baseline_min_kopecks / 100}
              onChange={(event) =>
                setPricing(
                  "baseline_min_kopecks",
                  Math.round(Number(event.target.value) * 100),
                )
              }
            />
          </label>
          <label>
            Базовая цена до, ₽
            <input
              required
              type="number"
              min={1}
              step={1000}
              value={value.pricing.baseline_max_kopecks / 100}
              onChange={(event) =>
                setPricing(
                  "baseline_max_kopecks",
                  Math.round(Number(event.target.value) * 100),
                )
              }
            />
          </label>
        </div>
        <div className="field-row">
          <label>
            Срок от, дней
            <input
              required
              type="number"
              min={1}
              max={3650}
              value={value.pricing.duration_min_days}
              onChange={(event) =>
                setPricing("duration_min_days", Number(event.target.value))
              }
            />
          </label>
          <label>
            Срок до, дней
            <input
              required
              type="number"
              min={1}
              max={3650}
              value={value.pricing.duration_max_days}
              onChange={(event) =>
                setPricing("duration_max_days", Number(event.target.value))
              }
            />
          </label>
        </div>
        <div className="field-row">
          <label>
            Категория (slug)
            <input
              placeholder="development"
              value={value.pricing.category_slug}
              onChange={(event) =>
                setPricing("category_slug", event.target.value)
              }
            />
          </label>
          <label>
            Навыки (slug через запятую)
            <input
              placeholder="flutter, react, go"
              value={value.pricing.skill_slugs.join(", ")}
              onChange={(event) =>
                setPricing(
                  "skill_slugs",
                  event.target.value
                    .split(",")
                    .map((item) => item.trim())
                    .filter(Boolean),
                )
              }
            />
          </label>
        </div>
      </section>
      <section className="calculator-builder__section">
        <div className="calculator-builder__title-row">
          <div>
            <h3>Вопросы и поправки</h3>
            <p>Итоговая поправка складывается из выбранных ответов.</p>
          </div>
          <button
            type="button"
            className="button button--quiet button--compact"
            onClick={addQuestion}
          >
            + Добавить вопрос
          </button>
        </div>
        <div className="calculator-question-list">
          {value.questions.map((question, index) => (
            <article
              className="calculator-question-editor"
              key={`${question.key}:${index}`}
            >
              <div className="calculator-question-editor__heading">
                <strong>Вопрос {index + 1}</strong>
                <button
                  type="button"
                  className="button button--danger button--compact"
                  onClick={() => removeQuestion(index)}
                >
                  Удалить
                </button>
              </div>
              <div className="field-row">
                <label>
                  Технический ключ
                  <input
                    required
                    pattern="[a-z][a-z0-9_]*"
                    value={question.key}
                    onChange={(event) =>
                      updateQuestion(index, {
                        ...question,
                        key: event.target.value
                          .toLowerCase()
                          .replace(/[^a-z0-9_]/g, ""),
                      })
                    }
                  />
                </label>
                <label>
                  Тип
                  <CustomSelect
                    value={question.type}
                    onChange={(event) =>
                      updateQuestion(index, {
                        ...question,
                        type: event.target.value as Question["type"],
                      })
                    }
                  >
                    <option value="SELECT">Выбор варианта</option>
                    <option value="NUMBER">Количество</option>
                    <option value="BOOLEAN">Да / нет</option>
                  </CustomSelect>
                </label>
              </div>
              <label>
                Текст вопроса
                <input
                  required
                  maxLength={200}
                  value={question.label}
                  onChange={(event) =>
                    updateQuestion(index, {
                      ...question,
                      label: event.target.value,
                    })
                  }
                />
              </label>
              {question.type === "BOOLEAN" ? (
                <label>
                  Надбавка при ответе «Да», %
                  <input
                    type="number"
                    min={0}
                    max={500}
                    step={1}
                    value={
                      (value.pricing.boolean_basis_points[question.key] ?? 0) /
                      100
                    }
                    onChange={(event) =>
                      setValue((current) => ({
                        ...current,
                        pricing: {
                          ...current.pricing,
                          boolean_basis_points: {
                            ...current.pricing.boolean_basis_points,
                            [question.key]: Math.round(
                              Number(event.target.value) * 100,
                            ),
                          },
                        },
                      }))
                    }
                  />
                </label>
              ) : question.type === "NUMBER" ? (
                <div className="field-row">
                  <label>
                    Минимум
                    <input
                      required
                      type="number"
                      min={0}
                      value={question.min ?? 0}
                      onChange={(event) =>
                        updateQuestion(index, {
                          ...question,
                          min: Number(event.target.value),
                        })
                      }
                    />
                  </label>
                  <label>
                    Максимум
                    <input
                      required
                      type="number"
                      min={question.min ?? 0}
                      value={question.max ?? 10}
                      onChange={(event) =>
                        updateQuestion(index, {
                          ...question,
                          max: Number(event.target.value),
                        })
                      }
                    />
                  </label>
                  <label>
                    Надбавка за единицу, %
                    <input
                      required
                      type="number"
                      min={0}
                      max={100}
                      step={0.1}
                      value={
                        (value.pricing.number_basis_points[question.key] ?? 0) /
                        100
                      }
                      onChange={(event) =>
                        setValue((current) => ({
                          ...current,
                          pricing: {
                            ...current.pricing,
                            number_basis_points: {
                              ...current.pricing.number_basis_points,
                              [question.key]: Math.round(
                                Number(event.target.value) * 100,
                              ),
                            },
                          },
                        }))
                      }
                    />
                  </label>
                </div>
              ) : (
                <div className="calculator-option-list">
                  {question.options?.map((option, optionIndex) => (
                    <div
                      className="calculator-option-row"
                      key={`${option.value}:${optionIndex}`}
                    >
                      <input
                        aria-label="Ключ варианта"
                        required
                        value={option.value}
                        onChange={(event) =>
                          optionChange(
                            index,
                            optionIndex,
                            {
                              ...option,
                              value: event.target.value
                                .toLowerCase()
                                .replace(/[^a-z0-9_]/g, ""),
                            },
                            (value.pricing.option_basis_points[
                              question.key
                            ]?.[option.value] ?? 0) / 100,
                          )
                        }
                      />
                      <input
                        aria-label="Название варианта"
                        required
                        value={option.label}
                        onChange={(event) =>
                          optionChange(
                            index,
                            optionIndex,
                            { ...option, label: event.target.value },
                            (value.pricing.option_basis_points[
                              question.key
                            ]?.[option.value] ?? 0) / 100,
                          )
                        }
                      />
                      <label>
                        + %
                        <input
                          aria-label="Надбавка в процентах"
                          type="number"
                          min={0}
                          max={500}
                          value={
                            (value.pricing.option_basis_points[
                              question.key
                            ]?.[option.value] ?? 0) / 100
                          }
                          onChange={(event) =>
                            optionChange(
                              index,
                              optionIndex,
                              option,
                              Number(event.target.value),
                            )
                          }
                        />
                      </label>
                      <button
                        type="button"
                        aria-label="Удалить вариант"
                        onClick={() =>
                          updateQuestion(index, {
                            ...question,
                            options: question.options?.filter(
                              (_, i) => i !== optionIndex,
                            ),
                          })
                        }
                      >
                        ×
                      </button>
                    </div>
                  ))}
                  <button
                    type="button"
                    className="button button--quiet button--compact"
                    onClick={() =>
                      updateQuestion(index, {
                        ...question,
                        options: [
                          ...(question.options ?? []),
                          {
                            value: `option_${(question.options?.length ?? 0) + 1}`,
                            label: "Новый вариант",
                          },
                        ],
                      })
                    }
                  >
                    + Вариант
                  </button>
                </div>
              )}
            </article>
          ))}
        </div>
      </section>
      <section className="calculator-builder__section">
        <label>
          Допущения результата (каждое с новой строки)
          <textarea
            rows={3}
            value={value.pricing.assumptions.join("\n")}
            onChange={(event) =>
              setPricing(
                "assumptions",
                event.target.value
                  .split("\n")
                  .map((item) => item.trim())
                  .filter(Boolean),
              )
            }
          />
        </label>
      </section>
      <div className="inline-actions">
        <button disabled={saving}>
          {saving
            ? "Сохраняем…"
            : creating
            ? "Создать калькулятор"
            : "Сохранить новой версией"}
        </button>
        {onCancel ? (
          <button
            type="button"
            className="button button--quiet"
            onClick={onCancel}
          >
            Отмена
          </button>
        ) : null}
      </div>
    </form>
  );
}

export default function CalculatorsAdminPage() {
  const [items, setItems] = useState<CalculatorDefinition[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [creating, setCreating] = useState(false);

  const load = useCallback(() => {
    setLoading(true);
    adminRequest<{ data: CalculatorDefinition[] }>("/api/v1/admin/calculators")
      .then((body) => {
        setItems(body.data ?? []);
        setError("");
      })
      .catch((reason) =>
        setError(
          reason instanceof Error
            ? reason.message
            : "Не удалось загрузить калькуляторы",
        ),
      )
      .finally(() => setLoading(false));
  }, []);

  useEffect(load, [load]);

  return (
    <>
      <AdminHeader
        title="Конструктор калькуляторов"
        description="Создавайте сколько угодно разделов. Цена считается от базовой вилки с понятными процентными поправками за ответы; каждое изменение сохраняется новой версией."
        actions={
          <button type="button" onClick={() => setCreating(true)}>
            + Новый калькулятор
          </button>
        }
      />
      {creating ? (
        <CalculatorEditor
          creating
          initial={emptyCalculator()}
          onCancel={() => setCreating(false)}
          onSaved={(saved) => {
            setItems((current) => [...current, saved]);
            setCreating(false);
          }}
        />
      ) : null}
      {loading ? (
        <AdminCalculatorsSkeleton />
      ) : error ? (
        <AdminError message={error} onRetry={load} />
      ) : !items.length && !creating ? (
        <div className="empty admin-empty">
          <h2>Калькуляторы пока не созданы</h2>
          <p>Нажмите «+ Новый калькулятор», чтобы настроить расчет стоимости для раздела.</p>
        </div>
      ) : (
        <div className="calculator-admin-grid">
          {items.map((item) => (
            <CalculatorEditor
              key={`${item.slug}:${item.version}`}
              initial={item}
              onSaved={(saved) =>
                setItems((current) =>
                  current.map((entry) =>
                    entry.slug === saved.slug ? saved : entry,
                  ),
                )
              }
            />
          ))}
        </div>
      )}
    </>
  );
}
