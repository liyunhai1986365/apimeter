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

import React, { useCallback, useMemo, useState } from 'react';
import GoCaptchaReact from 'go-captcha-react';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../helpers';

const CAPTCHA_IMAGE_WIDTH = 300;
const CAPTCHA_IMAGE_HEIGHT = 220;

const GoCaptcha = ({ scene, token, onVerify }) => {
  const { t } = useTranslation();
  const [visible, setVisible] = useState(false);
  const [challenge, setChallenge] = useState(null);
  const [verifying, setVerifying] = useState(false);

  const loadChallenge = useCallback(async () => {
    setChallenge(null);
    onVerify('');
    try {
      const response = await API.post('/api/captcha', { scene });
      if (response.data?.success && response.data?.data) {
        setChallenge(response.data.data);
        return;
      }
      showError(
        response.data?.message || t('Failed to load behavior verification'),
      );
    } catch (error) {
      showError(t('Failed to load behavior verification'));
    }
  }, [onVerify, scene, t]);

  const openCaptcha = () => {
    setVisible(true);
    void loadChallenge();
  };

  const verify = useCallback(
    async (position, reset) => {
      if (!challenge || verifying) return;
      setVerifying(true);
      try {
        const response = await API.post('/api/captcha/verify', {
          scene,
          captcha_key: challenge.captcha_key,
          x: position.x,
          y: position.y,
        });
        const proof = response.data?.data?.token;
        if (response.data?.success && proof) {
          onVerify(proof);
          setVisible(false);
          showSuccess(t('Behavior verification completed'));
          return;
        }
        reset();
        await loadChallenge();
      } catch (error) {
        reset();
        await loadChallenge();
      } finally {
        setVerifying(false);
      }
    },
    [challenge, loadChallenge, onVerify, scene, t, verifying],
  );

  const slideConfig = useMemo(
    () => ({
      width: CAPTCHA_IMAGE_WIDTH,
      height: CAPTCHA_IMAGE_HEIGHT,
      title: t('Drag the slider to fit the puzzle piece into the gap.'),
      showTheme: true,
      scope: true,
    }),
    [t],
  );
  const slideData = useMemo(
    () => ({
      image: challenge?.image || '',
      thumb: challenge?.tile || '',
      thumbX: challenge?.tile_x || 0,
      thumbY: challenge?.tile_y || 0,
      thumbWidth: challenge?.tile_width || 0,
      thumbHeight: challenge?.tile_height || 0,
    }),
    [challenge],
  );
  const slideEvents = useMemo(
    () => ({
      close: () => setVisible(false),
      refresh: () => void loadChallenge(),
      confirm: (position, reset) => void verify(position, reset),
    }),
    [loadChallenge, verify],
  );

  return (
    <>
      <div className='flex w-full justify-center [&>div]:!w-full'>
        <GoCaptchaReact.Button
          config={{ width: 330, height: 44 }}
          type={token ? 'success' : 'default'}
          title={token ? t('Verified') : t('Slide to verify')}
          clickEvent={openCaptcha}
        />
      </div>

      {visible && (
        <div
          className='fixed inset-0 z-[1001] flex items-center justify-center overflow-auto bg-black/50 p-2'
          role='dialog'
          aria-modal='true'
          aria-label={t('Behavior verification')}
        >
          <GoCaptchaReact.Slide
            config={slideConfig}
            data={slideData}
            events={slideEvents}
          />
        </div>
      )}
    </>
  );
};

export default GoCaptcha;
