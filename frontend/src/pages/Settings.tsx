import { useState, useEffect } from 'react'
import { useQuery, useMutation } from '@tanstack/react-query'
import {
  Box, Card, CardContent, Typography, LinearProgress, Stack, Button,
  TextField, Divider, Grid,
} from '@mui/material'
import { Save } from '@mui/icons-material'
import toast from 'react-hot-toast'
import api from '../api/client'

export default function Settings() {
  const { data, isLoading } = useQuery({
    queryKey: ['settings'],
    queryFn: () => api.get('/settings').then(r => r.data.data),
  })

  const s = (key: string) => data?.settings?.[key] ?? ''

  const [form, setForm] = useState({
    markup_pct: '',
    fixed_addon: '',
    exchange_rate: '',
    default_product_status: '',
    otapi_key: '',
    deepseek_key: '',
    cscart_url: '',
    cscart_key: '',
    prices_every_h: '',
    sync_every_h: '',
    delivery_cost_per_kg: '',
    usd_to_cny: '',
  })

  useEffect(() => {
    if (data) {
      setForm({
        markup_pct: String(data.markup_pct ?? ''),
        fixed_addon: String(data.fixed_addon ?? ''),
        exchange_rate: String(data.exchange_rate ?? ''),
        default_product_status: s('default_product_status') || 'A',
        otapi_key: data.otapi_key || '',
        deepseek_key: data.deepseek_key || '',
        cscart_url: data.cscart_url || '',
        cscart_key: data.cscart_key || '',
        prices_every_h: s('cron_prices_h') || '6',
        sync_every_h: s('cron_sync_h') || '12',
        delivery_cost_per_kg: s('delivery_cost_per_kg') || '2',
        usd_to_cny: s('usd_to_cny') || '7.2',
      })
    }
  }, [data])

  const savePricing = useMutation({
    mutationFn: () => api.post('/settings/pricing', {
      markup_pct: parseFloat(form.markup_pct),
      fixed_addon: parseFloat(form.fixed_addon),
      exchange_rate: parseFloat(form.exchange_rate),
    }),
    onSuccess: () => toast.success('Настройки цен сохранены'),
    onError: () => toast.error('Ошибка'),
  })

  const saveProduct = useMutation({
    mutationFn: () => api.post('/settings/product', {
      default_product_status: form.default_product_status,
    }),
    onSuccess: () => toast.success('Настройки сохранены'),
    onError: () => toast.error('Ошибка'),
  })

  const saveKeys = useMutation({
    mutationFn: () => api.post('/settings/keys', {
      otapi_key: form.otapi_key,
      deepseek_key: form.deepseek_key,
    }),
    onSuccess: () => toast.success('API ключи сохранены'),
    onError: () => toast.error('Ошибка'),
  })

  const saveCron = useMutation({
    mutationFn: () => api.post('/settings/cron', {
      prices_every_h: parseInt(form.prices_every_h) || 6,
      sync_every_h: parseInt(form.sync_every_h) || 12,
    }),
    onSuccess: () => toast.success('Расписание сохранено'),
    onError: () => toast.error('Ошибка'),
  })

  const saveDelivery = useMutation({
    mutationFn: () => api.post('/settings/delivery', {
      delivery_cost_per_kg: parseFloat(form.delivery_cost_per_kg),
      usd_to_cny: parseFloat(form.usd_to_cny),
    }),
    onSuccess: () => toast.success('Доставка сохранена'),
    onError: () => toast.error('Ошибка'),
  })

  const f = (field: string) => ({
    value: form[field as keyof typeof form],
    onChange: (e: any) => setForm(prev => ({ ...prev, [field]: e.target.value })),
  })

  if (isLoading) return <LinearProgress />

  return (
    <Box>
      <Typography sx={{ fontSize: 18, fontWeight: 700, mb: 2 }}>Настройки</Typography>

      <Grid container spacing={2}>
        <Grid size={{ xs: 12, md: 6 }}>
          <Card>
            <CardContent>
              <Typography sx={{ fontWeight: 600, mb: 2 }}>Ценообразование</Typography>
              <Stack spacing={2}>
                <TextField size="small" label="Наценка %" type="number" {...f('markup_pct')} />
                <TextField size="small" label="Фиксированная наценка (TMT)" type="number" {...f('fixed_addon')} />
                <TextField size="small" label="Курс CNY → TMT" type="number" {...f('exchange_rate')} />
                <Button variant="contained" startIcon={<Save />} onClick={() => savePricing.mutate()} disabled={savePricing.isPending}>
                  Сохранить
                </Button>
              </Stack>
            </CardContent>
          </Card>
        </Grid>

        <Grid size={{ xs: 12, md: 6 }}>
          <Card>
            <CardContent>
              <Typography sx={{ fontWeight: 600, mb: 2 }}>Товары</Typography>
              <Stack spacing={2}>
                <TextField size="small" label="Статус новых товаров (A/D/H)" {...f('default_product_status')} helperText="A = активный, D = скрытый, H = скрытый" />
                <Button variant="contained" startIcon={<Save />} onClick={() => saveProduct.mutate()} disabled={saveProduct.isPending}>
                  Сохранить
                </Button>
              </Stack>
            </CardContent>
          </Card>
        </Grid>

        <Grid size={{ xs: 12, md: 6 }}>
          <Card>
            <CardContent>
              <Typography sx={{ fontWeight: 600, mb: 2 }}>API ключи</Typography>
              <Stack spacing={2}>
                <TextField size="small" label="OTAPI Instance Key" type="password" {...f('otapi_key')} />
                <TextField size="small" label="DeepSeek API Key" type="password" {...f('deepseek_key')} />
                <Divider />
                <TextField size="small" label="CS-Cart API ключ" type="password" {...f('cscart_key')} />
                <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>
                  CS-Cart URL: {data?.cscart_url || '—'}
                </Typography>
                <Button variant="contained" startIcon={<Save />} onClick={() => saveKeys.mutate()} disabled={saveKeys.isPending}>
                  Сохранить ключи
                </Button>
              </Stack>
            </CardContent>
          </Card>
        </Grid>

        <Grid size={{ xs: 12, md: 6 }}>
          <Card>
            <CardContent>
              <Typography sx={{ fontWeight: 600, mb: 2 }}>Доставка</Typography>
              <Stack spacing={2}>
                <TextField size="small" label="Стоимость доставки ($/кг)" type="number" {...f('delivery_cost_per_kg')} />
                <TextField size="small" label="Курс USD → CNY" type="number" {...f('usd_to_cny')} />
                <Button variant="contained" startIcon={<Save />} onClick={() => saveDelivery.mutate()} disabled={saveDelivery.isPending}>
                  Сохранить
                </Button>
              </Stack>
            </CardContent>
          </Card>
        </Grid>

        <Grid size={{ xs: 12, md: 6 }}>
          <Card>
            <CardContent>
              <Typography sx={{ fontWeight: 600, mb: 2 }}>Расписание (Cron)</Typography>
              <Stack spacing={2}>
                <TextField size="small" label="Обновление цен (каждые N часов)" type="number" {...f('prices_every_h')} />
                <TextField size="small" label="Синхронизация (каждые N часов)" type="number" {...f('sync_every_h')} />
                <Button variant="contained" startIcon={<Save />} onClick={() => saveCron.mutate()} disabled={saveCron.isPending}>
                  Сохранить
                </Button>
              </Stack>
            </CardContent>
          </Card>
        </Grid>
      </Grid>
    </Box>
  )
}
