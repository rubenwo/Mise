import AppearanceSection from './AppearanceSection';
import BackgroundGenerationSettings from './BackgroundGenerationSettings';
import BackgroundTranslationSettings from './BackgroundTranslationSettings';
import FeatureStatusSection from './FeatureStatusSection';
import GeneralSettings from './GeneralSettings';
import ProvidersSection from './ProvidersSection';

export default function SettingsPage() {
  return (
    <div className="settings-page">
      <h2>Settings</h2>
      <AppearanceSection />
      <FeatureStatusSection />
      <ProvidersSection />
      <GeneralSettings />
      <BackgroundGenerationSettings />
      <BackgroundTranslationSettings />
    </div>
  );
}
