import { useState } from 'react'
import { useQuery, useMutation } from '@tanstack/react-query'
import {
  Box, Card, CardContent, Typography, LinearProgress, Stack, Button,
  Select, MenuItem, FormControl,
} from '@mui/material'
import { CloudUpload } from '@mui/icons-material'
import toast from 'react-hot-toast'
import api from '../api/client'
import type { CategoryMapping } from '../types'

export default function Push() {
  const [selectedCat, setSelectedCat] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['push'],
    queryFn: () => api.get('/push').then(r => r.data.data),
  })

  const pushMutation = useMutation({
    mutationFn: (catId: string) => api.post('/push/category', { category_id: catId }),
    onSuccess: r => toast.success(`Push запущен: ${r.data.data?.pushed ?? 0} товаров`),
    onError: () => toast.error('Ошибка push'),
  })

  const mappings: CategoryMapping[] = data?.mappings ?? []
  const settings = data?.settings ?? {}

  return (
    <Box>
      <Typography sx={{ fontSize: 20, fontWeight: 700, mb: 2 }}>Push в Wabrum</Typography>
      {isLoading && <LinearProgress />}

      <Card>
        <CardContent>
          <Typography sx={{ fontWeight: 600, mb: '4px' }}>Отправка товаров в CS-Cart</Typography>
          <Typography sx={{ fontSize: 12, color: 'text.secondary', mb: 2 }}>
            Отправляет новые (ещё не отправленные) товары из выбранной категории на wabrum.com.
            Статус товаров: <strong>{settings.default_product_status || 'A'}</strong>
          </Typography>
          <Stack spacing={2} sx={{ maxWidth: 500 }}>
            <FormControl size="small" fullWidth>
              <Select value={selectedCat} displayEmpty onChange={e => setSelectedCat(e.target.value as string)}>
                <MenuItem value=""><em>— выбери категорию —</em></MenuItem>
                {mappings.map(m => (
                  <MenuItem key={m.OTCategoryID} value={m.OTCategoryID}>
                    {m.OTCategoryID} → {m.CSCategoryName}{m.Notes && ` (${m.Notes})`}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
            <Button variant="contained" color="success" startIcon={<CloudUpload />}
              disabled={!selectedCat || pushMutation.isPending}
              onClick={() => pushMutation.mutate(selectedCat)}>
              Запустить Push
            </Button>
          </Stack>
        </CardContent>
      </Card>
    </Box>
  )
}
