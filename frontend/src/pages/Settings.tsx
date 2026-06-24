import { useState, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Box, Card, CardContent, Typography, LinearProgress, Stack, Button,
  TextField, Grid, InputAdornment, IconButton, Switch, FormControlLabel,
  Divider, Alert, Chip, FormGroup, FormControl, FormLabel, Checkbox,
} from '@mui/material'
import { Save, Visibility, VisibilityOff, Info, Sync } from '@mui/icons-material'
import toast from 'react-hot-toast'
import api from '../api/client'

const ALL_PROVIDERS = [
  { id: 'alibaba1688', label: '1688.com', desc: 'Основной китайский B2B маркетплейс' },
  { id: 'taobao',      label: 'Taobao',   desc: 'Китайский B2C маркетплейс' },
  { id: 'jd',          label: 'JD.com',   desc: 'Крупный китайский ретейлер' },
  { id: 'poizon',      label: 'Poizon',   desc: 'Кроссовки и одежда (Dewu)' },
]

function KeyField({ label, fieldKey, form, setForm, helperText }: {
  label: string
  fieldKey: string
  form: Record<string, string>
  setForm: (fn: (p: Record<string, string>) => Record<string, string>) => void
  helperText?: string
}) {
  const [show, setShow] = useState(false)
  return (
    <TextField
      size="small"
      label={label}
      type={show ? 'text' : 'password'}
      value={form[fieldKey] ?? ''}
      onChange={e => setForm(p => ({ ...p, [fieldKey]: e.target.value }))}
      helperText={helperText}
      slotProps={{
        input: {
          endAdornment: (
            <InputAdornment position="end">
              <IconButton size="small" onClick={() => setShow(v => !v)} edge="end">
                {show ? <VisibilityOff fontSize="small" /> : <Visibility fontSize="small" />}
              </IconButton>
            </InputAdornment>
          ),
        },
      }}
    />
  )
}

function SectionTitle({ children }: { children: React.ReactNode }) {
  return <Typography sx={{ fontWeight: 600, mb: 1.5 }}>{children}</Typography>
}

function FormulaBox({ lines }: { lines: string[] }) {
  return (
    <Alert severity="info" icon={<Info fontSize="small" />} sx={{ py: 0.5, '& .MuiAlert-message': { width: '100%' } }}>
      <Stack spacing={0.3}>
        {lines.map((l, i) => (
          <Typography key={i} sx={{ fontSize: 12, fontFamily: 'monospace', lineHeight: 1.5 }}>{l}</Typography>
        ))}
      </Stack>
    </Alert>
  )
}

export default function Settings() {
  const qc = useQueryClient()
  const { data, isLoading } = useQuery({
    queryKey: ['settings'],
    queryFn: () => api.get('/settings').then(r => r.data.data),
  })

  const [form, setForm] = useState<Record<string, string>>({})
  const [deliveryIncluded, setDeliveryIncluded] = useState(false)
  const [providers, setProviders] = useState<Set<string>>(new Set(['alibaba1688']))
  const [defaultPrompt, setDefaultPrompt] = useState('')

  useEffect(() => {
    if (!data) return
    const s = (k: string) => String(data.settings?.[k] ?? data[k] ?? '')
    setForm({
      markup_pct:           String(data.markup_pct ?? ''),
      fixed_addon:          String(data.fixed_addon ?? ''),
      exchange_rate:        String(data.exchange_rate ?? ''),
      default_product_status: s('default_product_status') || 'D',
      otapi_key:            data.otapi_key || '',
      deepseek_key:         data.deepseek_key || '',
      deepseek_base_url:    data.deepseek_base_url || 'https://api.deepseek.com',
      cscart_key:           data.cscart_key || '',
      cscart_url:           data.cscart_url || '',
      cscart_email:         data.cscart_email || '',
      prices_every_h:       s('cron_prices_h') || '6',
      sync_every_h:         s('cron_sync_h') || '12',
      delivery_cost_per_kg: s('delivery_cost_per_kg') || '2',
      usd_to_cny:           s('usd_to_cny') || '7.2',
      // Пред-заполняем поле дефолтным промптом, если кастомный не задан —
      // чтобы он был виден как редактируемый текст, а не только в placeholder.
      deepseek_prompt:      s('deepseek_prompt') || data.deepseek_prompt_default || '',
    })
    setDeliveryIncluded(data.delivery_included === true)
    if (data.deepseek_prompt_default) setDefaultPrompt(data.deepseek_prompt_default)
    const pStr = data.enabled_providers || s('enabled_providers') || 'alibaba1688'
    setProviders(new Set(pStr.split(',').map((x: string) => x.trim()).filter(Boolean)))
  }, [data])

  const f = (key: string) => ({
    value: form[key] ?? '',
    onChange: (e: any) => setForm(p => ({ ...p, [key]: e.target.value })),
  })

  const mut = (fn: () => Promise<any>, msg: string) => ({
    mutationFn: fn,
    onSuccess: () => { toast.success(msg); qc.invalidateQueries({ queryKey: ['settings'] }) },
    onError: () => toast.error('Ошибка сохранения'),
  })

  const syncCSFeatures = useMutation({
    mutationFn: () => api.post('/settings/sync-cs-features', {}),
    onSuccess: (r) => toast.success(`Загружено: ${r.data.data.features_count} характеристик, ${r.data.data.variants_count} вариантов`),
    onError: () => toast.error('Ошибка загрузки характеристик CS-Cart'),
  })

  const savePricing   = useMutation(mut(() => api.post('/settings/pricing', {
    markup_pct:    parseFloat(form.markup_pct) || 0,
    fixed_addon:   parseFloat(form.fixed_addon) || 0,
    exchange_rate: parseFloat(form.exchange_rate) || 0,
  }), 'Цены сохранены'))

  const saveProduct   = useMutation(mut(() => api.post('/settings/product', {
    default_product_status: form.default_product_status,
  }), 'Настройки товаров сохранены'))

  const saveKeys      = useMutation(mut(() => api.post('/settings/keys', {
    otapi_key:    form.otapi_key,
    deepseek_key: form.deepseek_key,
    cscart_key:   form.cscart_key,
    cscart_url:   form.cscart_url,
    cscart_email: form.cscart_email,
  }), 'API ключи сохранены'))

  const saveCron      = useMutation(mut(() => api.post('/settings/cron', {
    prices_every_h: parseInt(form.prices_every_h) || 6,
    sync_every_h:   parseInt(form.sync_every_h) || 12,
  }), 'Расписание сохранено'))

  const saveDelivery  = useMutation(mut(() => api.post('/settings/delivery', {
    delivery_cost_per_kg: parseFloat(form.delivery_cost_per_kg) || 0,
    usd_to_cny:           parseFloat(form.usd_to_cny) || 0,
    delivery_included:    deliveryIncluded,
  }), 'Доставка сохранена'))

  const savePrompt    = useMutation(mut(() => api.post('/settings/prompt', {
    prompt: form.deepseek_prompt,
  }), 'Промпт сохранён'))

  const saveProviders = useMutation(mut(() => api.post('/settings/providers', {
    providers: Array.from(providers),
  }), 'Провайдеры сохранены'))

  if (isLoading) return <LinearProgress />

  // Live price preview
  const markupPct  = parseFloat(form.markup_pct) || 0
  const fixedAddon = parseFloat(form.fixed_addon) || 0
  const rate       = parseFloat(form.exchange_rate) || 0
  const delivCost  = parseFloat(form.delivery_cost_per_kg) || 0
  const usdToCny   = parseFloat(form.usd_to_cny) || 1
  const exampleCny   = 100
  const exampleGrams = 500
  const delivCnyPerG = (delivCost / usdToCny) / 1000
  const delivAdd     = deliveryIncluded ? delivCnyPerG * exampleGrams : 0
  const basePrice    = (exampleCny + delivAdd) * rate
  const finalPrice   = basePrice * (1 + markupPct / 100) + fixedAddon

  return (
    <Box>
      <Typography sx={{ fontSize: 18, fontWeight: 700, mb: 2 }}>Настройки</Typography>

      <Grid container spacing={2}>

        {/* API Keys */}
        <Grid size={{ xs: 12, md: 6 }}>
          <Card>
            <CardContent>
              <SectionTitle>API ключи</SectionTitle>
              <Stack spacing={2}>
                <KeyField label="OTAPI Instance Key" fieldKey="otapi_key" form={form} setForm={setForm}
                  helperText="Ключ инстанса OT Commerce / 1688" />
                <Divider />
                <KeyField label="DeepSeek API Key" fieldKey="deepseek_key" form={form} setForm={setForm}
                  helperText="sk-... (для AI-переводов названий и описаний)" />
                <TextField size="small" label="DeepSeek Base URL" {...f('deepseek_base_url')}
                  helperText="https://api.deepseek.com" />
                <Divider />
                <TextField size="small" label="CS-Cart URL" {...f('cscart_url')}
                  helperText="https://example.com" />
                <TextField size="small" label="CS-Cart Email" {...f('cscart_email')}
                  helperText="api@example.com" />
                <KeyField label="CS-Cart API Key" fieldKey="cscart_key" form={form} setForm={setForm} />
                <Button variant="contained" startIcon={<Save />}
                  onClick={() => saveKeys.mutate()} disabled={saveKeys.isPending}>
                  Сохранить ключи
                </Button>
                <Divider />
                <Box>
                  <Typography sx={{ fontSize: 12, fontWeight: 600, mb: 0.5 }}>Характеристики CS-Cart</Typography>
                  <Typography sx={{ fontSize: 12, color: 'text.secondary', mb: 1 }}>
                    Загружает список всех характеристик и их допустимых значений из CS-Cart в локальный кеш.
                    Нужно для нормализации атрибутов товаров перед пушем. Выполнять при изменении характеристик на сайте.
                  </Typography>
                  <Button variant="outlined" startIcon={<Sync />}
                    onClick={() => syncCSFeatures.mutate()} disabled={syncCSFeatures.isPending}>
                    {syncCSFeatures.isPending ? 'Загрузка...' : 'Синхронизировать характеристики CS-Cart'}
                  </Button>
                </Box>
              </Stack>
            </CardContent>
          </Card>
        </Grid>

        {/* Pricing */}
        <Grid size={{ xs: 12, md: 6 }}>
          <Card>
            <CardContent>
              <SectionTitle>Ценообразование</SectionTitle>
              <Stack spacing={2}>
                <FormulaBox lines={[
                  'базовая = цена_1688 (CNY) × курс_CNY→TMT',
                  'итого   = базовая × (1 + наценка%) + фикс_наценка',
                  '',
                  `Пример: 100 CNY × ${rate} × (1 + ${markupPct}%) + ${fixedAddon} = ${finalPrice.toFixed(2)} TMT`,
                ]} />
                <TextField size="small" label="Наценка %" type="number" {...f('markup_pct')}
                  helperText="Процент сверху базовой цены (напр. 35 = +35%)" />
                <TextField size="small" label="Фиксированная наценка (TMT)" type="number" {...f('fixed_addon')}
                  helperText="Добавляется после % наценки (напр. 5 TMT на каждый товар)" />
                <TextField size="small" label="Курс CNY → TMT" type="number" {...f('exchange_rate')}
                  helperText="Текущий курс: 1 юань = ? манат" />
                <Button variant="contained" startIcon={<Save />}
                  onClick={() => savePricing.mutate()} disabled={savePricing.isPending}>
                  Сохранить
                </Button>
              </Stack>
            </CardContent>
          </Card>
        </Grid>

        {/* Delivery */}
        <Grid size={{ xs: 12, md: 6 }}>
          <Card>
            <CardContent>
              <SectionTitle>Доставка из Китая</SectionTitle>
              <Stack spacing={2}>
                <FormulaBox lines={[
                  'стоимость_доставки_CNY = ($/кг ÷ USD→CNY) ÷ 1000 × вес_граммов',
                  '',
                  `Пример: ${exampleGrams}г — доставка = (${delivCost} ÷ ${usdToCny}) ÷ 1000 × ${exampleGrams} = ${delivAdd.toFixed(3)} CNY`,
                  deliveryIncluded ? '→ добавляется к цене товара перед наценкой' : '→ не включается в цену',
                ]} />
                <TextField size="small" label="Стоимость доставки ($/кг)" type="number"
                  {...f('delivery_cost_per_kg')} helperText="Цена логистики за 1 кг в долларах США" />
                <TextField size="small" label="Курс USD → CNY" type="number" {...f('usd_to_cny')}
                  helperText="Для пересчёта стоимости доставки из $ в CNY" />
                <FormControlLabel
                  control={<Switch checked={deliveryIncluded} onChange={e => setDeliveryIncluded(e.target.checked)} />}
                  label={
                    <Box>
                      <Typography sx={{ fontSize: 14 }}>Включить доставку в цену товара</Typography>
                      <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>
                        {deliveryIncluded ? 'Стоимость доставки добавляется к цене' : 'Цена без учёта доставки'}
                      </Typography>
                    </Box>
                  }
                />
                <Button variant="contained" startIcon={<Save />}
                  onClick={() => saveDelivery.mutate()} disabled={saveDelivery.isPending}>
                  Сохранить
                </Button>
              </Stack>
            </CardContent>
          </Card>
        </Grid>

        {/* Products */}
        <Grid size={{ xs: 12, md: 6 }}>
          <Card>
            <CardContent>
              <SectionTitle>Товары</SectionTitle>
              <Stack spacing={2}>
                <FormulaBox lines={[
                  'A = Active (виден покупателям)',
                  'D = Disabled (скрыт, не отображается)',
                  'H = Hidden (скрыт в поиске, доступен по прямой ссылке)',
                ]} />
                <TextField size="small" label="Статус новых товаров" {...f('default_product_status')}
                  helperText="A = активный, D = скрытый, H = скрытый в поиске" />
                <Button variant="contained" startIcon={<Save />}
                  onClick={() => saveProduct.mutate()} disabled={saveProduct.isPending}>
                  Сохранить
                </Button>
              </Stack>
            </CardContent>
          </Card>
        </Grid>

        {/* Cron */}
        <Grid size={{ xs: 12, md: 6 }}>
          <Card>
            <CardContent>
              <SectionTitle>Расписание (Cron)</SectionTitle>
              <Stack spacing={2}>
                <FormulaBox lines={[
                  'Обновление цен - периодически обновляет цены из 1688',
                  'Синхронизация - загружает новые/изменённые товары',
                  '0 = задача отключена',
                ]} />
                <TextField size="small" label="Обновление цен (каждые N часов)" type="number"
                  {...f('prices_every_h')} helperText="0 = отключено" />
                <TextField size="small" label="Синхронизация товаров (каждые N часов)" type="number"
                  {...f('sync_every_h')} helperText="0 = отключено" />
                <Button variant="contained" startIcon={<Save />}
                  onClick={() => saveCron.mutate()} disabled={saveCron.isPending}>
                  Сохранить
                </Button>
              </Stack>
            </CardContent>
          </Card>
        </Grid>

        {/* Providers */}
        <Grid size={{ xs: 12, md: 6 }}>
          <Card>
            <CardContent>
              <SectionTitle>Провайдеры товаров</SectionTitle>
              <Stack spacing={2}>
                <Typography sx={{ fontSize: 12, color: 'text.secondary' }}>
                  Выберите платформы, с которых будут загружаться товары
                </Typography>
                <FormControl component="fieldset">
                  <FormLabel component="legend" sx={{ fontSize: 12, mb: 1 }}>Активные источники</FormLabel>
                  <FormGroup>
                    {ALL_PROVIDERS.map(p => (
                      <FormControlLabel
                        key={p.id}
                        control={
                          <Checkbox
                            size="small"
                            checked={providers.has(p.id)}
                            onChange={e => {
                              const next = new Set(providers)
                              if (e.target.checked) next.add(p.id)
                              else next.delete(p.id)
                              setProviders(next)
                            }}
                          />
                        }
                        label={
                          <Box>
                            <Typography sx={{ fontSize: 13, fontWeight: 500 }}>{p.label}</Typography>
                            <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>{p.desc}</Typography>
                          </Box>
                        }
                        sx={{ mb: 0.5 }}
                      />
                    ))}
                  </FormGroup>
                </FormControl>
                <Box sx={{ display: 'flex', gap: 0.5, flexWrap: 'wrap' }}>
                  {Array.from(providers).map(p => {
                    const meta = ALL_PROVIDERS.find(x => x.id === p)
                    return <Chip key={p} label={meta?.label ?? p} size="small" color="primary" />
                  })}
                  {providers.size === 0 && (
                    <Typography sx={{ fontSize: 12, color: 'warning.main' }}>Ни один провайдер не выбран</Typography>
                  )}
                </Box>
                <Button variant="contained" startIcon={<Save />}
                  onClick={() => saveProviders.mutate()} disabled={saveProviders.isPending}>
                  Сохранить
                </Button>
              </Stack>
            </CardContent>
          </Card>
        </Grid>

        {/* DeepSeek Prompt */}
        <Grid size={{ xs: 12 }}>
          <Card>
            <CardContent>
              <SectionTitle>DeepSeek - системный промпт для генерации описаний</SectionTitle>
              <Stack spacing={2}>
                <Typography sx={{ fontSize: 12, color: 'text.secondary' }}>
                  Этот промпт используется при автоматической генерации русских названий и описаний товаров.
                  DeepSeek получает китайское название + атрибуты товара и генерирует текст для CS-Cart.
                  Поле уже заполнено дефолтным промптом — можно редактировать. Если очистить — снова применится встроенный дефолт.
                </Typography>
                <TextField
                  size="small"
                  label="Системный промпт (редактируемый)"
                  multiline
                  rows={12}
                  {...f('deepseek_prompt')}
                  placeholder={defaultPrompt}
                  helperText="Редактируй под себя. Пустое поле = встроенный дефолтный промпт."
                />
                {defaultPrompt && (
                  <Button size="small" variant="text" sx={{ alignSelf: 'flex-start' }}
                    onClick={() => setForm({ ...form, deepseek_prompt: defaultPrompt })}>
                    ↺ Вернуть дефолтный промпт
                  </Button>
                )}
                <Button variant="contained" startIcon={<Save />}
                  onClick={() => savePrompt.mutate()} disabled={savePrompt.isPending}>
                  Сохранить промпт
                </Button>
              </Stack>
            </CardContent>
          </Card>
        </Grid>

      </Grid>
    </Box>
  )
}
