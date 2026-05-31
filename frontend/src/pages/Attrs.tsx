import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Box, Card, CardContent, Typography, LinearProgress, Stack, Button,
  Table, TableBody, TableCell, TableContainer, TableHead, TableRow,
} from '@mui/material'
import { Translate } from '@mui/icons-material'
import toast from 'react-hot-toast'
import api from '../api/client'
import type { AttrTranslation } from '../types'

function formatTs(ts: number) {
  if (!ts) return '-'
  return new Date(ts * 1000).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
}

export default function Attrs() {
  const qc = useQueryClient()

  const { data, isLoading } = useQuery({
    queryKey: ['attrs'],
    queryFn: () => api.get('/attrs').then(r => r.data.data),
  })

  const translateMutation = useMutation({
    mutationFn: () => api.post('/attrs/translate'),
    onSuccess: (r) => {
      toast.success(`Переведено: ${r.data.data?.translated ?? 0} атрибутов`)
      qc.invalidateQueries({ queryKey: ['attrs'] })
    },
    onError: () => toast.error('Ошибка перевода'),
  })

  const total: number = data?.total ?? 0
  const translated: number = data?.translated ?? 0
  const pending: number = data?.pending ?? 0
  const attrs: AttrTranslation[] = data?.recent ?? []
  const pct = total > 0 ? Math.round(translated / total * 100) : 0

  return (
    <Box>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
        <Typography sx={{ fontSize: 18, fontWeight: 700 }}>Атрибуты товаров</Typography>
        <Button
          variant="contained" startIcon={<Translate />}
          onClick={() => translateMutation.mutate()}
          disabled={translateMutation.isPending || pending === 0}
        >
          Перевести через DeepSeek{pending > 0 && ` (${pending})`}
        </Button>
      </Box>

      {isLoading && <LinearProgress />}

      <Stack direction="row" spacing={2} sx={{ mb: 3 }}>
        {[
          { label: 'Всего', value: total, color: 'text.primary' },
          { label: 'Переведено', value: translated, color: 'success.main' },
          { label: 'Ожидают', value: pending, color: 'warning.main' },
        ].map(s => (
          <Card key={s.label} sx={{ flex: 1 }}>
            <CardContent>
              <Typography sx={{ fontSize: 11, textTransform: 'uppercase', color: 'text.secondary', letterSpacing: '0.05em' }}>
                {s.label}
              </Typography>
              <Typography sx={{ fontSize: 28, fontWeight: 700, color: s.color }}>{s.value}</Typography>
            </CardContent>
          </Card>
        ))}
      </Stack>

      <Card sx={{ mb: 3 }}>
        <CardContent>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: '4px' }}>
            <Typography sx={{ fontSize: 13 }}>Прогресс перевода</Typography>
            <Typography sx={{ fontSize: 13, fontWeight: 600 }}>{pct}%</Typography>
          </Box>
          <Box sx={{ height: 8, borderRadius: 4, background: '#e9ecef', overflow: 'hidden' }}>
            <Box sx={{ height: '100%', width: `${pct}%`, background: 'primary.main', bgcolor: 'primary.main', transition: 'width 0.3s' }} />
          </Box>
        </CardContent>
      </Card>

      <Card>
        <CardContent sx={{ pb: '12px !important' }}>
          <Typography sx={{ fontWeight: 600, mb: '12px' }}>Последние переводы</Typography>
          <TableContainer>
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell>Атрибут (ZH)</TableCell>
                  <TableCell>Атрибут (RU)</TableCell>
                  <TableCell>Значение (ZH)</TableCell>
                  <TableCell>Значение (RU)</TableCell>
                  <TableCell>Дата</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {attrs.map(a => (
                  <TableRow key={`${a.pid}:${a.vid}`} hover>
                    <TableCell><Typography sx={{ fontSize: 12 }}>{a.property_name_zh}</Typography></TableCell>
                    <TableCell><Typography sx={{ fontSize: 12, fontWeight: 500 }}>{a.property_name_ru}</Typography></TableCell>
                    <TableCell><Typography sx={{ fontSize: 12 }}>{a.value_zh}</Typography></TableCell>
                    <TableCell><Typography sx={{ fontSize: 12, fontWeight: 500 }}>{a.value_ru}</Typography></TableCell>
                    <TableCell><Typography sx={{ fontSize: 11, color: 'text.secondary' }}>{formatTs(a.translated_at)}</Typography></TableCell>
                  </TableRow>
                ))}
                {!isLoading && attrs.length === 0 && (
                  <TableRow>
                    <TableCell colSpan={5} align="center" sx={{ py: 4, color: 'text.secondary' }}>
                      Нет переведённых атрибутов
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </TableContainer>
        </CardContent>
      </Card>
    </Box>
  )
}
