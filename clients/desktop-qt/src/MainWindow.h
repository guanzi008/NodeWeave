#pragma once

#include "ControlPlaneClient.h"

#include <QComboBox>
#include <QJsonArray>
#include <QJsonObject>
#include <QLineEdit>
#include <QMainWindow>
#include <QPlainTextEdit>
#include <QPushButton>
#include <QSettings>
#include <QTableWidget>

class MainWindow : public QMainWindow {
    Q_OBJECT

public:
    explicit MainWindow(QWidget *parent = nullptr);
    ~MainWindow() override = default;

protected:
    void closeEvent(QCloseEvent *event) override;

private:
    void buildUi();
    void wireSignals();
    void loadSettings();
    void saveSettings();
    void loadForwardingSettings();
    void saveForwardingSettings();
    void appendLog(const QString &line);
    void updateNodesTable(const QJsonArray &items);
    void refreshLocalSerialDevices();
    void refreshLocalUsbDevices();
    void applySelectedSerialDevice(bool onlyIfEmpty = false);
    void applySelectedUsbDevice(bool onlyIfEmpty = false);
    void updateSerialRulePreview();
    void updateUsbRulePreview();
    void refreshSerialTable();
    void refreshUsbTable();
    void syncForwardingPreview(QPlainTextEdit *editor, const QJsonArray &entries);
    void exportEntries(const QString &title, const QString &suggestedName, const QJsonArray &entries);
    void exportAgentConfigSnippet(const QString &platform, const QString &suggestedName);
    void importAgentConfigSnippet(const QString &title);
    bool importEntriesFromFile(const QString &title, const QString &objectKey, QJsonArray *entries);
    void loadJsonPreviewFromFile(const QString &title, QPlainTextEdit *editor);
    QJsonObject buildRegistrationPayload() const;
    QJsonObject buildSerialPayload() const;
    QJsonObject buildUsbPayload() const;
    QJsonObject buildAgentConfigSnippet(const QString &platform) const;
    QString buildForwardingSessionId(const QString &prefix,
                                     const QString &nodeId,
                                     const QString &peerNodeId,
                                     const QString &left,
                                     const QString &right,
                                     const QString &transport) const;
    static QString sanitizeIdPart(const QString &value);

    ControlPlaneClient *m_client;
    QSettings m_settings;

    QLineEdit *m_serverUrlEdit;
    QLineEdit *m_emailEdit;
    QLineEdit *m_passwordEdit;
    QLineEdit *m_tokenEdit;
    QPushButton *m_healthButton;
    QPushButton *m_loginButton;
    QPushButton *m_nodesButton;

    QLineEdit *m_registrationTokenEdit;
    QLineEdit *m_deviceNameEdit;
    QLineEdit *m_platformEdit;
    QLineEdit *m_versionEdit;
    QLineEdit *m_publicKeyEdit;
    QLineEdit *m_capabilitiesEdit;
    QPushButton *m_registerButton;

    QLineEdit *m_serialNodeIdEdit;
    QLineEdit *m_serialPeerNodeIdEdit;
    QComboBox *m_serialDetectedCombo;
    QLineEdit *m_serialLocalPortEdit;
    QLineEdit *m_serialRemotePortEdit;
    QLineEdit *m_serialBaudRateEdit;
    QLineEdit *m_serialTransportEdit;
    QPushButton *m_serialDetectButton;
    QPushButton *m_serialUseDetectedButton;
    QPushButton *m_serialAddButton;
    QPushButton *m_serialRemoveButton;
    QPushButton *m_serialExportButton;
    QPushButton *m_serialImportButton;
    QPushButton *m_serialLoadReportButton;
    QTableWidget *m_serialTable;
    QPlainTextEdit *m_serialJsonText;
    QPlainTextEdit *m_serialReportText;
    QPlainTextEdit *m_serialRuleText;

    QLineEdit *m_usbNodeIdEdit;
    QLineEdit *m_usbPeerNodeIdEdit;
    QComboBox *m_usbDetectedCombo;
    QLineEdit *m_usbLocalBusEdit;
    QLineEdit *m_usbLocalDeviceEdit;
    QLineEdit *m_usbLocalVendorEdit;
    QLineEdit *m_usbLocalProductEdit;
    QLineEdit *m_usbLocalInterfaceEdit;
    QLineEdit *m_usbRemoteBusEdit;
    QLineEdit *m_usbRemoteDeviceEdit;
    QLineEdit *m_usbRemoteVendorEdit;
    QLineEdit *m_usbRemoteProductEdit;
    QLineEdit *m_usbRemoteInterfaceEdit;
    QLineEdit *m_usbTransportEdit;
    QPushButton *m_usbDetectButton;
    QPushButton *m_usbUseDetectedButton;
    QPushButton *m_usbAddButton;
    QPushButton *m_usbRemoveButton;
    QPushButton *m_usbExportButton;
    QPushButton *m_usbImportButton;
    QPushButton *m_usbLoadReportButton;
    QTableWidget *m_usbTable;
    QPlainTextEdit *m_usbJsonText;
    QPlainTextEdit *m_usbReportText;
    QPlainTextEdit *m_usbRuleText;

    QTableWidget *m_nodesTable;
    QPlainTextEdit *m_overviewText;
    QPlainTextEdit *m_logText;
    QPlainTextEdit *m_registrationText;
    QPushButton *m_exportLinuxAgentButton;
    QPushButton *m_exportWindowsAgentButton;
    QPushButton *m_importLinuxAgentButton;
    QPushButton *m_importWindowsAgentButton;

    QJsonArray m_serialEntries;
    QJsonArray m_usbEntries;
    QJsonArray m_localSerialDevices;
    QJsonArray m_localUsbDevices;
};
