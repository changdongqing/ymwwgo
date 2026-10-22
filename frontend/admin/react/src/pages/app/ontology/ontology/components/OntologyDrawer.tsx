import { useEffect, useRef, useState } from 'react';
import type { ProFormInstance } from '@ant-design/pro-components';
import { DrawerForm, ProFormSwitch, ProFormText, ProFormTextArea } from '@ant-design/pro-components';
import { App } from 'antd';
import { useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';

import { useCreateOntology, useUpdateOntology } from '@/api/hooks/ontology';
import type { ontologyservicev1_Ontology } from '@/api/generated/admin/service/v1';

interface OntologyDrawerProps {
  open: boolean;
  mode: 'create' | 'edit';
  data?: ontologyservicev1_Ontology | null;
  onClose: () => void;
  onSuccess: () => void;
}

/**
 * 本体文档编辑/创建抽屉（DrawerForm + formRef，无 useForm，镜像 DictTypeDrawer）。
 * base_iri 创建后不可变（源修订 #1：变更命名空间会使既有实体 IRI 语义漂移）——
 * 编辑态禁用输入；前端做"尾部补 #"的即时预览。
 */
const OntologyDrawer: React.FC<OntologyDrawerProps> = ({ open, mode, data, onClose, onSuccess }) => {
  const { t } = useTranslation('ontology');
  const formRef = useRef<ProFormInstance>(null);
  const queryClient = useQueryClient();
  const { message } = App.useApp();

  const [baseIriPreview, setBaseIriPreview] = useState('');

  useEffect(() => {
    if (open && mode === 'edit' && data) {
      setTimeout(() => {
        formRef.current?.setFieldsValue({
          ontologyName: data.ontologyName || '',
          ontologyIri: data.ontologyIri || '',
          baseIri: data.baseIri || '',
          description: data.description || '',
          isEnabled: data.isEnabled ?? true,
        });
        setBaseIriPreview(data.baseIri || '');
      }, 0);
    }
    if (open && mode === 'create') {
      setBaseIriPreview('');
    }
  }, [open, mode, data]);

  const createMutation = useCreateOntology({
    onSuccess: () => {
      message.success(t('ontology.createSuccess'));
      queryClient.invalidateQueries({ queryKey: ['listOntologies'] });
      queryClient.invalidateQueries({ queryKey: ['simpleOntologies'] });
      onSuccess();
      onClose();
    },
    onError: (error: Error) => message.error(error.message || t('ontology.createFailed')),
  });

  const updateMutation = useUpdateOntology({
    onSuccess: () => {
      message.success(t('ontology.updateSuccess'));
      queryClient.invalidateQueries({ queryKey: ['listOntologies'] });
      queryClient.invalidateQueries({ queryKey: ['simpleOntologies'] });
      onSuccess();
      onClose();
    },
    onError: (error: Error) => message.error(error.message || t('ontology.updateFailed')),
  });

  const handleSubmit = async (values: Record<string, any>) => {
    try {
      if (mode === 'edit' && data?.id) {
        // base_iri 不可变：不进提交体（编辑态已禁用，这里双保险）
        const { baseIri: _ignored, ontologyIri: _iri, ...rest } = values;
        await updateMutation.mutateAsync({ id: data.id, values: rest });
      } else {
        await createMutation.mutateAsync({ values });
      }
      return true;
    } catch {
      return false;
    }
  };

  return (
    <DrawerForm
      formRef={formRef}
      title={mode === 'create' ? t('ontology.create') : t('ontology.edit')}
      open={open}
      onOpenChange={(visible) => {
        if (!visible) {
          formRef.current?.resetFields();
          onClose();
        }
      }}
      initialValues={{ isEnabled: true }}
      onFinish={handleSubmit}
      submitter={{
        searchConfig: {
          submitText: t('common:button.submit'),
          resetText: t('common:button.cancel'),
        },
        submitButtonProps: {
          loading: createMutation.isPending || updateMutation.isPending,
        },
        resetButtonProps: { onClick: onClose },
      }}
      drawerProps={{ destroyOnHidden: true, onClose, size: 600 }}
    >
      <ProFormText
        name="ontologyName"
        label={t('ontology.ontologyName')}
        placeholder={t('ontology.ontologyNamePlaceholder')}
        rules={[{ required: true, message: t('ontology.requiredOntologyName') }]}
        fieldProps={{ allowClear: true }}
      />

      <ProFormText
        name="ontologyIri"
        label={t('ontology.ontologyIri')}
        placeholder={t('ontology.ontologyIriPlaceholder')}
        rules={[
          { required: true, message: t('ontology.requiredOntologyIri') },
          {
            validator: (_rule, value: string) => {
              if (!value || /^https?:\/\/.+/.test(value.trim())) return Promise.resolve();
              return Promise.reject(new Error(t('ontology.invalidIri')));
            },
          },
        ]}
        fieldProps={{ allowClear: true, disabled: mode === 'edit' }}
        extra={t('ontology.ontologyIriImmutable')}
      />

      <ProFormText
        name="baseIri"
        label={t('ontology.baseIri')}
        placeholder={t('ontology.baseIriPlaceholder')}
        rules={[
          { required: true, message: t('ontology.requiredBaseIri') },
          {
            validator: (_rule, value: string) => {
              if (!value || /^https?:\/\/.+/.test(value.trim())) return Promise.resolve();
              return Promise.reject(new Error(t('ontology.invalidIri')));
            },
          },
        ]}
        fieldProps={{
          allowClear: true,
          disabled: mode === 'edit',
          onChange: (e: React.ChangeEvent<HTMLInputElement>) => {
            const value = e.target.value?.trim() ?? '';
            setBaseIriPreview(value && !value.endsWith('#') ? `${value}#` : value);
          },
        }}
        extra={
          baseIriPreview
            ? `${t('ontology.baseIriPreview')}${baseIriPreview}`
            : t('ontology.baseIriHint')
        }
      />

      <ProFormTextArea
        name="description"
        label={t('ontology.description')}
        placeholder={t('ontology.descriptionPlaceholder')}
        fieldProps={{ autoSize: { minRows: 2, maxRows: 4 }, maxLength: 512, showCount: true }}
      />

      <ProFormSwitch name="isEnabled" label={t('ontology.isEnabled')} />
    </DrawerForm>
  );
};

export default OntologyDrawer;
