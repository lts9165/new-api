/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useEffect, useState, useRef } from 'react';
import { Modal, Card, Spin, Typography, Button, Toast } from '@douyinfe/semi-ui';
import { CheckCircle, XCircle, Clock, Smartphone } from 'lucide-react';
import { API } from '../../../helpers';

const { Text, Title } = Typography;

const QRCodePaymentModal = ({
  visible,
  onCancel,
  paymentData,
  t,
}) => {
  const [status, setStatus] = useState('pending'); // pending, success, failed, timeout
  const [countdown, setCountdown] = useState(300); // 5分钟倒计时
  const pollingInterval = useRef(null);
  const countdownInterval = useRef(null);

  // 轮询订单状态
  const pollOrderStatus = async () => {
    if (!paymentData?.trade_no) return;

    try {
      const res = await API.get(`/api/user/topup/status/${paymentData.trade_no}`);
      const { success, data } = res.data;

      if (success && data) {
        if (data.status === 'success') {
          setStatus('success');
          stopPolling();
          Toast.success(t('支付成功！'));
          setTimeout(() => {
            onCancel();
            window.location.reload(); // 刷新页面更新余额
          }, 2000);
        } else if (data.status === 'failed') {
          setStatus('failed');
          stopPolling();
        }
      }
    } catch (err) {
      console.error('查询订单状态失败:', err);
    }
  };

  // 停止轮询
  const stopPolling = () => {
    if (pollingInterval.current) {
      clearInterval(pollingInterval.current);
      pollingInterval.current = null;
    }
    if (countdownInterval.current) {
      clearInterval(countdownInterval.current);
      countdownInterval.current = null;
    }
  };

  // 开始轮询
  useEffect(() => {
    if (visible && paymentData) {
      setStatus('pending');
      setCountdown(300);

      // 每3秒轮询一次订单状态
      pollingInterval.current = setInterval(pollOrderStatus, 3000);

      // 倒计时
      countdownInterval.current = setInterval(() => {
        setCountdown((prev) => {
          if (prev <= 1) {
            setStatus('timeout');
            stopPolling();
            return 0;
          }
          return prev - 1;
        });
      }, 1000);

      return () => {
        stopPolling();
      };
    }
  }, [visible, paymentData]);

  // 格式化倒计时
  const formatCountdown = (seconds) => {
    const minutes = Math.floor(seconds / 60);
    const secs = seconds % 60;
    return `${minutes}:${secs.toString().padStart(2, '0')}`;
  };

  // 渲染状态图标
  const renderStatusIcon = () => {
    switch (status) {
      case 'success':
        return <CheckCircle size={64} color='#52c41a' />;
      case 'failed':
        return <XCircle size={64} color='#ff4d4f' />;
      case 'timeout':
        return <Clock size={64} color='#faad14' />;
      default:
        return <Spin size='large' />;
    }
  };

  // 渲染状态文本
  const renderStatusText = () => {
    switch (status) {
      case 'success':
        return t('支付成功！');
      case 'failed':
        return t('支付失败');
      case 'timeout':
        return t('二维码已过期');
      default:
        return t('等待支付中...');
    }
  };

  return (
    <Modal
      title={
        <div className='flex items-center gap-2'>
          <Smartphone size={18} />
          {t('扫码支付')}
        </div>
      }
      visible={visible}
      onCancel={onCancel}
      footer={null}
      centered
      width={420}
      maskClosable={false}
    >
      <div className='flex flex-col items-center py-6'>
        {/* 状态显示 */}
        {status !== 'pending' ? (
          <div className='flex flex-col items-center gap-4'>
            {renderStatusIcon()}
            <Title heading={4}>{renderStatusText()}</Title>
            {status === 'timeout' && (
              <Text type='secondary'>{t('请关闭弹窗重新发起支付')}</Text>
            )}
            {status === 'failed' && (
              <Text type='secondary'>{t('支付失败，请重试')}</Text>
            )}
          </div>
        ) : (
          <>
            {/* 二维码卡片 */}
            <Card
              className='!rounded-xl shadow-md'
              bodyStyle={{
                padding: '24px',
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
              }}
            >
              {/* 二维码图片 */}
              {paymentData?.qr_code ? (
                <div className='bg-white p-4 rounded-lg'>
                  <img
                    src={paymentData.qr_code}
                    alt='Payment QR Code'
                    style={{
                      width: '240px',
                      height: '240px',
                      display: 'block',
                    }}
                  />
                </div>
              ) : (
                <div
                  style={{
                    width: '240px',
                    height: '240px',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                  }}
                >
                  <Spin size='large' />
                </div>
              )}

              {/* 提示文本 */}
              <div className='text-center mt-4'>
                <Text strong className='text-base'>
                  {t('请使用支付APP扫码支付')}
                </Text>
              </div>
            </Card>

            {/* 倒计时 */}
            <div className='mt-6 flex items-center gap-2'>
              <Clock size={16} color='#666' />
              <Text type='secondary'>
                {t('二维码有效期')}: {formatCountdown(countdown)}
              </Text>
            </div>

            {/* 手机端支付按钮 */}
            {paymentData?.pay_url && (
              <Button
                className='mt-4'
                theme='solid'
                onClick={() => {
                  window.open(paymentData.pay_url, '_blank');
                }}
              >
                {t('手机浏览器打开支付')}
              </Button>
            )}
          </>
        )}

        {/* 关闭按钮 */}
        <Button
          className='mt-6'
          onClick={onCancel}
          type='tertiary'
        >
          {t('关闭')}
        </Button>
      </div>
    </Modal>
  );
};

export default QRCodePaymentModal;
