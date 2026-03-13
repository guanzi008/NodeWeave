#include "MainWindow.h"

#include "LocalDeviceInventory.h"

#include <QCloseEvent>
#include <QDateTime>
#include <QFile>
#include <QFileDialog>
#include <QFormLayout>
#include <QHBoxLayout>
#include <QGridLayout>
#include <QGroupBox>
#include <QHeaderView>
#include <QJsonDocument>
#include <QJsonValue>
#include <QLabel>
#include <QStatusBar>
#include <QTabWidget>
#include <QTableWidgetItem>
#include <QVBoxLayout>
#include <QWidget>

MainWindow::MainWindow(QWidget *parent)
    : QMainWindow(parent),
      m_client(new ControlPlaneClient(this)),
      m_settings(QStringLiteral("NodeWeave"), QStringLiteral("DesktopQtClient")),
      m_serverUrlEdit(nullptr),
      m_emailEdit(nullptr),
      m_passwordEdit(nullptr),
      m_tokenEdit(nullptr),
      m_healthButton(nullptr),
      m_loginButton(nullptr),
      m_nodesButton(nullptr),
      m_registrationTokenEdit(nullptr),
      m_deviceNameEdit(nullptr),
      m_platformEdit(nullptr),
      m_versionEdit(nullptr),
      m_publicKeyEdit(nullptr),
      m_capabilitiesEdit(nullptr),
      m_registerButton(nullptr),
      m_serialNodeIdEdit(nullptr),
      m_serialPeerNodeIdEdit(nullptr),
      m_serialDetectedCombo(nullptr),
      m_serialLocalPortEdit(nullptr),
      m_serialRemotePortEdit(nullptr),
      m_serialBaudRateEdit(nullptr),
      m_serialTransportEdit(nullptr),
      m_serialDetectButton(nullptr),
      m_serialUseDetectedButton(nullptr),
      m_serialAddButton(nullptr),
      m_serialRemoveButton(nullptr),
      m_serialExportButton(nullptr),
      m_serialImportButton(nullptr),
      m_serialLoadReportButton(nullptr),
      m_serialTable(nullptr),
      m_serialJsonText(nullptr),
      m_serialReportText(nullptr),
      m_serialRuleText(nullptr),
      m_usbNodeIdEdit(nullptr),
      m_usbPeerNodeIdEdit(nullptr),
      m_usbDetectedCombo(nullptr),
      m_usbLocalBusEdit(nullptr),
      m_usbLocalDeviceEdit(nullptr),
      m_usbLocalVendorEdit(nullptr),
      m_usbLocalProductEdit(nullptr),
      m_usbLocalInterfaceEdit(nullptr),
      m_usbRemoteBusEdit(nullptr),
      m_usbRemoteDeviceEdit(nullptr),
      m_usbRemoteVendorEdit(nullptr),
      m_usbRemoteProductEdit(nullptr),
      m_usbRemoteInterfaceEdit(nullptr),
      m_usbTransportEdit(nullptr),
      m_usbDetectButton(nullptr),
      m_usbUseDetectedButton(nullptr),
      m_usbAddButton(nullptr),
      m_usbRemoveButton(nullptr),
      m_usbExportButton(nullptr),
      m_usbImportButton(nullptr),
      m_usbLoadReportButton(nullptr),
      m_usbTable(nullptr),
      m_usbJsonText(nullptr),
      m_usbReportText(nullptr),
      m_usbRuleText(nullptr),
      m_nodesTable(nullptr),
      m_overviewText(nullptr),
      m_logText(nullptr),
      m_registrationText(nullptr),
      m_exportLinuxAgentButton(nullptr),
      m_exportWindowsAgentButton(nullptr),
      m_importLinuxAgentButton(nullptr),
      m_importWindowsAgentButton(nullptr) {
    buildUi();
    loadSettings();
    wireSignals();
    statusBar()->showMessage(QStringLiteral("就绪"));
}

void MainWindow::closeEvent(QCloseEvent *event) {
    saveSettings();
    QMainWindow::closeEvent(event);
}

void MainWindow::buildUi() {
    setWindowTitle(QStringLiteral("NodeWeave 客户端"));
    resize(1360, 900);

    QWidget *central = new QWidget(this);
    auto *rootLayout = new QVBoxLayout(central);
    rootLayout->setContentsMargins(12, 12, 12, 12);
    rootLayout->setSpacing(12);

    auto *connectionGroup = new QGroupBox(QStringLiteral("控制面"), central);
    auto *connectionLayout = new QGridLayout(connectionGroup);

    m_serverUrlEdit = new QLineEdit(connectionGroup);
    m_emailEdit = new QLineEdit(connectionGroup);
    m_passwordEdit = new QLineEdit(connectionGroup);
    m_passwordEdit->setEchoMode(QLineEdit::Password);
    m_tokenEdit = new QLineEdit(connectionGroup);
    m_healthButton = new QPushButton(QStringLiteral("健康检查"), connectionGroup);
    m_loginButton = new QPushButton(QStringLiteral("登录"), connectionGroup);
    m_nodesButton = new QPushButton(QStringLiteral("刷新节点"), connectionGroup);

    connectionLayout->addWidget(new QLabel(QStringLiteral("服务器地址"), connectionGroup), 0, 0);
    connectionLayout->addWidget(m_serverUrlEdit, 0, 1, 1, 3);
    connectionLayout->addWidget(new QLabel(QStringLiteral("邮箱"), connectionGroup), 1, 0);
    connectionLayout->addWidget(m_emailEdit, 1, 1);
    connectionLayout->addWidget(new QLabel(QStringLiteral("密码"), connectionGroup), 1, 2);
    connectionLayout->addWidget(m_passwordEdit, 1, 3);
    connectionLayout->addWidget(new QLabel(QStringLiteral("访问令牌"), connectionGroup), 2, 0);
    connectionLayout->addWidget(m_tokenEdit, 2, 1, 1, 3);
    connectionLayout->addWidget(m_healthButton, 3, 1);
    connectionLayout->addWidget(m_loginButton, 3, 2);
    connectionLayout->addWidget(m_nodesButton, 3, 3);

    auto *tabs = new QTabWidget(central);

    QWidget *overviewTab = new QWidget(tabs);
    auto *overviewLayout = new QVBoxLayout(overviewTab);
    m_overviewText = new QPlainTextEdit(overviewTab);
    m_overviewText->setReadOnly(true);
    m_logText = new QPlainTextEdit(overviewTab);
    m_logText->setReadOnly(true);
    auto *exportButtons = new QHBoxLayout();
    m_exportLinuxAgentButton = new QPushButton(QStringLiteral("导出 Linux Agent 配置片段"), overviewTab);
    m_exportWindowsAgentButton = new QPushButton(QStringLiteral("导出 Windows Agent 配置片段"), overviewTab);
    m_importLinuxAgentButton = new QPushButton(QStringLiteral("导入 Linux Agent 配置片段"), overviewTab);
    m_importWindowsAgentButton = new QPushButton(QStringLiteral("导入 Windows Agent 配置片段"), overviewTab);
    exportButtons->addWidget(m_exportLinuxAgentButton);
    exportButtons->addWidget(m_exportWindowsAgentButton);
    exportButtons->addWidget(m_importLinuxAgentButton);
    exportButtons->addWidget(m_importWindowsAgentButton);
    overviewLayout->addWidget(new QLabel(QStringLiteral("概览"), overviewTab));
    overviewLayout->addWidget(m_overviewText, 2);
    overviewLayout->addLayout(exportButtons);
    overviewLayout->addWidget(new QLabel(QStringLiteral("事件日志"), overviewTab));
    overviewLayout->addWidget(m_logText, 1);

    QWidget *nodesTab = new QWidget(tabs);
    auto *nodesLayout = new QVBoxLayout(nodesTab);
    m_nodesTable = new QTableWidget(nodesTab);
    m_nodesTable->setColumnCount(7);
    m_nodesTable->setHorizontalHeaderLabels({
        QStringLiteral("节点 ID"),
        QStringLiteral("设备 ID"),
        QStringLiteral("虚拟 IP"),
        QStringLiteral("状态"),
        QStringLiteral("Relay 区域"),
        QStringLiteral("最后心跳"),
        QStringLiteral("端点"),
    });
    m_nodesTable->horizontalHeader()->setStretchLastSection(true);
    m_nodesTable->horizontalHeader()->setSectionResizeMode(QHeaderView::ResizeToContents);
    nodesLayout->addWidget(m_nodesTable);

    QWidget *registrationTab = new QWidget(tabs);
    auto *registrationLayout = new QVBoxLayout(registrationTab);
    auto *registrationForm = new QFormLayout();
    m_registrationTokenEdit = new QLineEdit(registrationTab);
    m_deviceNameEdit = new QLineEdit(registrationTab);
    m_platformEdit = new QLineEdit(registrationTab);
    m_versionEdit = new QLineEdit(registrationTab);
    m_publicKeyEdit = new QLineEdit(registrationTab);
    m_capabilitiesEdit = new QLineEdit(registrationTab);
    m_registerButton = new QPushButton(QStringLiteral("注册设备"), registrationTab);
    m_registrationText = new QPlainTextEdit(registrationTab);
    m_registrationText->setReadOnly(true);

    registrationForm->addRow(QStringLiteral("注册令牌"), m_registrationTokenEdit);
    registrationForm->addRow(QStringLiteral("设备名称"), m_deviceNameEdit);
    registrationForm->addRow(QStringLiteral("平台"), m_platformEdit);
    registrationForm->addRow(QStringLiteral("版本"), m_versionEdit);
    registrationForm->addRow(QStringLiteral("公钥"), m_publicKeyEdit);
    registrationForm->addRow(QStringLiteral("能力列表（CSV）"), m_capabilitiesEdit);
    registrationLayout->addLayout(registrationForm);
    registrationLayout->addWidget(m_registerButton);
    registrationLayout->addWidget(m_registrationText, 1);

    QWidget *serialTab = new QWidget(tabs);
    auto *serialLayout = new QVBoxLayout(serialTab);
    auto *serialForm = new QFormLayout();
    m_serialNodeIdEdit = new QLineEdit(serialTab);
    m_serialPeerNodeIdEdit = new QLineEdit(serialTab);
    m_serialDetectedCombo = new QComboBox(serialTab);
    m_serialLocalPortEdit = new QLineEdit(serialTab);
    m_serialRemotePortEdit = new QLineEdit(serialTab);
    m_serialBaudRateEdit = new QLineEdit(serialTab);
    m_serialTransportEdit = new QLineEdit(serialTab);
    m_serialBaudRateEdit->setText(QStringLiteral("115200"));
    m_serialTransportEdit->setText(QStringLiteral("tcp-encap"));
    m_serialDetectButton = new QPushButton(QStringLiteral("扫描本机串口"), serialTab);
    m_serialUseDetectedButton = new QPushButton(QStringLiteral("使用选中串口"), serialTab);
    m_serialAddButton = new QPushButton(QStringLiteral("添加串口映射"), serialTab);
    m_serialRemoveButton = new QPushButton(QStringLiteral("删除选中项"), serialTab);
    m_serialExportButton = new QPushButton(QStringLiteral("导出 JSON"), serialTab);
    m_serialImportButton = new QPushButton(QStringLiteral("导入 JSON"), serialTab);
    m_serialLoadReportButton = new QPushButton(QStringLiteral("加载报告"), serialTab);
    m_serialTable = new QTableWidget(serialTab);
    m_serialTable->setColumnCount(7);
    m_serialTable->setHorizontalHeaderLabels({
        QStringLiteral("会话 ID"),
        QStringLiteral("本端节点"),
        QStringLiteral("对端节点"),
        QStringLiteral("本地串口"),
        QStringLiteral("远端串口"),
        QStringLiteral("波特率"),
        QStringLiteral("传输方式"),
    });
    m_serialTable->horizontalHeader()->setStretchLastSection(true);
    m_serialTable->horizontalHeader()->setSectionResizeMode(QHeaderView::ResizeToContents);
    m_serialJsonText = new QPlainTextEdit(serialTab);
    m_serialJsonText->setReadOnly(true);
    m_serialReportText = new QPlainTextEdit(serialTab);
    m_serialReportText->setReadOnly(true);
    m_serialRuleText = new QPlainTextEdit(serialTab);
    m_serialRuleText->setReadOnly(true);

    auto *serialDiscoveryRow = new QHBoxLayout();
    serialDiscoveryRow->addWidget(new QLabel(QStringLiteral("本机串口"), serialTab));
    serialDiscoveryRow->addWidget(m_serialDetectedCombo, 1);
    serialDiscoveryRow->addWidget(m_serialDetectButton);
    serialDiscoveryRow->addWidget(m_serialUseDetectedButton);
    serialForm->addRow(QStringLiteral("本端节点 ID"), m_serialNodeIdEdit);
    serialForm->addRow(QStringLiteral("对端节点 ID"), m_serialPeerNodeIdEdit);
    serialForm->addRow(QStringLiteral("本地串口"), m_serialLocalPortEdit);
    serialForm->addRow(QStringLiteral("远端串口"), m_serialRemotePortEdit);
    serialForm->addRow(QStringLiteral("波特率"), m_serialBaudRateEdit);
    serialForm->addRow(QStringLiteral("传输方式"), m_serialTransportEdit);
    auto *serialButtons = new QHBoxLayout();
    serialButtons->addWidget(m_serialAddButton);
    serialButtons->addWidget(m_serialRemoveButton);
    serialButtons->addWidget(m_serialExportButton);
    serialButtons->addWidget(m_serialImportButton);
    serialButtons->addWidget(m_serialLoadReportButton);
    serialLayout->addLayout(serialDiscoveryRow);
    serialLayout->addLayout(serialForm);
    serialLayout->addLayout(serialButtons);
    serialLayout->addWidget(m_serialTable, 2);
    serialLayout->addWidget(new QLabel(QStringLiteral("串口驱动 / 规则"), serialTab));
    serialLayout->addWidget(m_serialRuleText, 1);
    serialLayout->addWidget(new QLabel(QStringLiteral("串口 JSON 配置"), serialTab));
    serialLayout->addWidget(m_serialJsonText, 1);
    serialLayout->addWidget(new QLabel(QStringLiteral("串口报告"), serialTab));
    serialLayout->addWidget(m_serialReportText, 1);

    QWidget *usbTab = new QWidget(tabs);
    auto *usbLayout = new QVBoxLayout(usbTab);
    auto *usbForm = new QGridLayout();
    m_usbNodeIdEdit = new QLineEdit(usbTab);
    m_usbPeerNodeIdEdit = new QLineEdit(usbTab);
    m_usbDetectedCombo = new QComboBox(usbTab);
    m_usbLocalBusEdit = new QLineEdit(usbTab);
    m_usbLocalDeviceEdit = new QLineEdit(usbTab);
    m_usbLocalVendorEdit = new QLineEdit(usbTab);
    m_usbLocalProductEdit = new QLineEdit(usbTab);
    m_usbLocalInterfaceEdit = new QLineEdit(usbTab);
    m_usbRemoteBusEdit = new QLineEdit(usbTab);
    m_usbRemoteDeviceEdit = new QLineEdit(usbTab);
    m_usbRemoteVendorEdit = new QLineEdit(usbTab);
    m_usbRemoteProductEdit = new QLineEdit(usbTab);
    m_usbRemoteInterfaceEdit = new QLineEdit(usbTab);
    m_usbTransportEdit = new QLineEdit(usbTab);
    m_usbTransportEdit->setText(QStringLiteral("usbip-encap"));
    m_usbDetectButton = new QPushButton(QStringLiteral("扫描本机 USB 设备"), usbTab);
    m_usbUseDetectedButton = new QPushButton(QStringLiteral("使用选中设备"), usbTab);
    m_usbAddButton = new QPushButton(QStringLiteral("添加 USB 映射"), usbTab);
    m_usbRemoveButton = new QPushButton(QStringLiteral("删除选中项"), usbTab);
    m_usbExportButton = new QPushButton(QStringLiteral("导出 JSON"), usbTab);
    m_usbImportButton = new QPushButton(QStringLiteral("导入 JSON"), usbTab);
    m_usbLoadReportButton = new QPushButton(QStringLiteral("加载报告"), usbTab);
    m_usbTable = new QTableWidget(usbTab);
    m_usbTable->setColumnCount(6);
    m_usbTable->setHorizontalHeaderLabels({
        QStringLiteral("会话 ID"),
        QStringLiteral("本端节点"),
        QStringLiteral("对端节点"),
        QStringLiteral("本地设备"),
        QStringLiteral("远端设备"),
        QStringLiteral("传输方式"),
    });
    m_usbTable->horizontalHeader()->setStretchLastSection(true);
    m_usbTable->horizontalHeader()->setSectionResizeMode(QHeaderView::ResizeToContents);
    m_usbJsonText = new QPlainTextEdit(usbTab);
    m_usbJsonText->setReadOnly(true);
    m_usbReportText = new QPlainTextEdit(usbTab);
    m_usbReportText->setReadOnly(true);
    m_usbRuleText = new QPlainTextEdit(usbTab);
    m_usbRuleText->setReadOnly(true);

    auto *usbDiscoveryRow = new QHBoxLayout();
    usbDiscoveryRow->addWidget(new QLabel(QStringLiteral("本机 USB 设备"), usbTab));
    usbDiscoveryRow->addWidget(m_usbDetectedCombo, 1);
    usbDiscoveryRow->addWidget(m_usbDetectButton);
    usbDiscoveryRow->addWidget(m_usbUseDetectedButton);
    int row = 0;
    usbForm->addWidget(new QLabel(QStringLiteral("本端节点 ID"), usbTab), row, 0);
    usbForm->addWidget(m_usbNodeIdEdit, row, 1);
    usbForm->addWidget(new QLabel(QStringLiteral("对端节点 ID"), usbTab), row, 2);
    usbForm->addWidget(m_usbPeerNodeIdEdit, row, 3);
    ++row;
    usbForm->addWidget(new QLabel(QStringLiteral("本地总线"), usbTab), row, 0);
    usbForm->addWidget(m_usbLocalBusEdit, row, 1);
    usbForm->addWidget(new QLabel(QStringLiteral("本地设备号"), usbTab), row, 2);
    usbForm->addWidget(m_usbLocalDeviceEdit, row, 3);
    ++row;
    usbForm->addWidget(new QLabel(QStringLiteral("本地厂商 ID"), usbTab), row, 0);
    usbForm->addWidget(m_usbLocalVendorEdit, row, 1);
    usbForm->addWidget(new QLabel(QStringLiteral("本地产品 ID"), usbTab), row, 2);
    usbForm->addWidget(m_usbLocalProductEdit, row, 3);
    ++row;
    usbForm->addWidget(new QLabel(QStringLiteral("本地接口"), usbTab), row, 0);
    usbForm->addWidget(m_usbLocalInterfaceEdit, row, 1);
    usbForm->addWidget(new QLabel(QStringLiteral("传输方式"), usbTab), row, 2);
    usbForm->addWidget(m_usbTransportEdit, row, 3);
    ++row;
    usbForm->addWidget(new QLabel(QStringLiteral("远端总线"), usbTab), row, 0);
    usbForm->addWidget(m_usbRemoteBusEdit, row, 1);
    usbForm->addWidget(new QLabel(QStringLiteral("远端设备号"), usbTab), row, 2);
    usbForm->addWidget(m_usbRemoteDeviceEdit, row, 3);
    ++row;
    usbForm->addWidget(new QLabel(QStringLiteral("远端厂商 ID"), usbTab), row, 0);
    usbForm->addWidget(m_usbRemoteVendorEdit, row, 1);
    usbForm->addWidget(new QLabel(QStringLiteral("远端产品 ID"), usbTab), row, 2);
    usbForm->addWidget(m_usbRemoteProductEdit, row, 3);
    ++row;
    usbForm->addWidget(new QLabel(QStringLiteral("远端接口"), usbTab), row, 0);
    usbForm->addWidget(m_usbRemoteInterfaceEdit, row, 1);

    auto *usbButtons = new QHBoxLayout();
    usbButtons->addWidget(m_usbAddButton);
    usbButtons->addWidget(m_usbRemoveButton);
    usbButtons->addWidget(m_usbExportButton);
    usbButtons->addWidget(m_usbImportButton);
    usbButtons->addWidget(m_usbLoadReportButton);
    usbLayout->addLayout(usbDiscoveryRow);
    usbLayout->addLayout(usbForm);
    usbLayout->addLayout(usbButtons);
    usbLayout->addWidget(m_usbTable, 2);
    usbLayout->addWidget(new QLabel(QStringLiteral("USB 驱动 / 规则"), usbTab));
    usbLayout->addWidget(m_usbRuleText, 1);
    usbLayout->addWidget(new QLabel(QStringLiteral("USB JSON 配置"), usbTab));
    usbLayout->addWidget(m_usbJsonText, 1);
    usbLayout->addWidget(new QLabel(QStringLiteral("USB 报告"), usbTab));
    usbLayout->addWidget(m_usbReportText, 1);

    tabs->addTab(overviewTab, QStringLiteral("概览"));
    tabs->addTab(nodesTab, QStringLiteral("节点"));
    tabs->addTab(registrationTab, QStringLiteral("注册"));
    tabs->addTab(serialTab, QStringLiteral("串口"));
    tabs->addTab(usbTab, QStringLiteral("USB 设备"));

    rootLayout->addWidget(connectionGroup);
    rootLayout->addWidget(tabs, 1);

    setCentralWidget(central);
}

void MainWindow::wireSignals() {
    connect(m_serverUrlEdit, &QLineEdit::editingFinished, this, [this]() {
        m_client->setBaseUrl(QUrl::fromUserInput(m_serverUrlEdit->text().trimmed()));
    });
    connect(m_tokenEdit, &QLineEdit::editingFinished, this, [this]() {
        m_client->setAccessToken(m_tokenEdit->text().trimmed());
    });

    connect(m_healthButton, &QPushButton::clicked, this, [this]() {
        m_client->setBaseUrl(QUrl::fromUserInput(m_serverUrlEdit->text().trimmed()));
        m_client->fetchHealth();
    });
    connect(m_loginButton, &QPushButton::clicked, this, [this]() {
        m_client->setBaseUrl(QUrl::fromUserInput(m_serverUrlEdit->text().trimmed()));
        m_client->login(m_emailEdit->text(), m_passwordEdit->text());
    });
    connect(m_nodesButton, &QPushButton::clicked, this, [this]() {
        m_client->setBaseUrl(QUrl::fromUserInput(m_serverUrlEdit->text().trimmed()));
        m_client->setAccessToken(m_tokenEdit->text().trimmed());
        m_client->listNodes();
    });
    connect(m_registerButton, &QPushButton::clicked, this, [this]() {
        m_client->setBaseUrl(QUrl::fromUserInput(m_serverUrlEdit->text().trimmed()));
        m_client->registerDevice(buildRegistrationPayload());
    });
    connect(m_serialDetectButton, &QPushButton::clicked, this, [this]() {
        refreshLocalSerialDevices();
    });
    connect(m_serialUseDetectedButton, &QPushButton::clicked, this, [this]() {
        applySelectedSerialDevice(false);
    });
    connect(m_serialDetectedCombo, QOverload<int>::of(&QComboBox::currentIndexChanged), this, [this](int) {
        updateSerialRulePreview();
    });
    connect(m_usbDetectButton, &QPushButton::clicked, this, [this]() {
        refreshLocalUsbDevices();
    });
    connect(m_usbUseDetectedButton, &QPushButton::clicked, this, [this]() {
        applySelectedUsbDevice(false);
    });
    connect(m_usbDetectedCombo, QOverload<int>::of(&QComboBox::currentIndexChanged), this, [this](int) {
        updateUsbRulePreview();
    });
    connect(m_exportLinuxAgentButton, &QPushButton::clicked, this, [this]() {
        exportAgentConfigSnippet(QStringLiteral("linux-agent"), QStringLiteral("linux-agent-forwarding.snippet.json"));
    });
    connect(m_exportWindowsAgentButton, &QPushButton::clicked, this, [this]() {
        exportAgentConfigSnippet(QStringLiteral("windows-agent"), QStringLiteral("windows-agent-forwarding.snippet.json"));
    });
    connect(m_importLinuxAgentButton, &QPushButton::clicked, this, [this]() {
        importAgentConfigSnippet(QStringLiteral("导入 Linux Agent 转发配置片段"));
    });
    connect(m_importWindowsAgentButton, &QPushButton::clicked, this, [this]() {
        importAgentConfigSnippet(QStringLiteral("导入 Windows Agent 转发配置片段"));
    });

    connect(m_serialAddButton, &QPushButton::clicked, this, [this]() {
        const QJsonObject entry = buildSerialPayload();
        if (entry.isEmpty()) {
            appendLog(QStringLiteral("串口映射需要填写本地串口和远端串口名称"));
            return;
        }
        m_serialEntries.append(entry);
        refreshSerialTable();
        saveForwardingSettings();
        appendLog(QStringLiteral("已添加串口转发映射 %1").arg(entry.value(QStringLiteral("session_id")).toString()));
    });
    connect(m_serialRemoveButton, &QPushButton::clicked, this, [this]() {
        const int row = m_serialTable->currentRow();
        if (row < 0 || row >= m_serialEntries.size()) {
            return;
        }
        m_serialEntries.removeAt(row);
        refreshSerialTable();
        saveForwardingSettings();
    });
    connect(m_serialExportButton, &QPushButton::clicked, this, [this]() {
        exportEntries(QStringLiteral("导出串口转发配置"), QStringLiteral("serial-forwards.json"), m_serialEntries);
    });
    connect(m_serialImportButton, &QPushButton::clicked, this, [this]() {
        if (importEntriesFromFile(QStringLiteral("导入串口转发配置"), QStringLiteral("serial_forwards"), &m_serialEntries)) {
            refreshSerialTable();
            saveForwardingSettings();
        }
    });
    connect(m_serialLoadReportButton, &QPushButton::clicked, this, [this]() {
        loadJsonPreviewFromFile(QStringLiteral("加载串口转发报告"), m_serialReportText);
    });

    connect(m_usbAddButton, &QPushButton::clicked, this, [this]() {
        const QJsonObject entry = buildUsbPayload();
        if (entry.isEmpty()) {
            appendLog(QStringLiteral("USB 映射至少需要填写一组本地设备和远端设备标识"));
            return;
        }
        m_usbEntries.append(entry);
        refreshUsbTable();
        saveForwardingSettings();
        appendLog(QStringLiteral("已添加 USB 转发映射 %1").arg(entry.value(QStringLiteral("session_id")).toString()));
    });
    connect(m_usbRemoveButton, &QPushButton::clicked, this, [this]() {
        const int row = m_usbTable->currentRow();
        if (row < 0 || row >= m_usbEntries.size()) {
            return;
        }
        m_usbEntries.removeAt(row);
        refreshUsbTable();
        saveForwardingSettings();
    });
    connect(m_usbExportButton, &QPushButton::clicked, this, [this]() {
        exportEntries(QStringLiteral("导出 USB 转发配置"), QStringLiteral("usb-forwards.json"), m_usbEntries);
    });
    connect(m_usbImportButton, &QPushButton::clicked, this, [this]() {
        if (importEntriesFromFile(QStringLiteral("导入 USB 转发配置"), QStringLiteral("usb_forwards"), &m_usbEntries)) {
            refreshUsbTable();
            saveForwardingSettings();
        }
    });
    connect(m_usbLoadReportButton, &QPushButton::clicked, this, [this]() {
        loadJsonPreviewFromFile(QStringLiteral("加载 USB 转发报告"), m_usbReportText);
    });

    connect(m_client, &ControlPlaneClient::healthReady, this, [this](const QJsonObject &payload) {
        m_overviewText->setPlainText(QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Indented)));
        appendLog(QStringLiteral("健康检查成功"));
        statusBar()->showMessage(QStringLiteral("健康检查正常"), 3000);
    });
    connect(m_client, &ControlPlaneClient::loginReady, this, [this](const QString &accessToken, const QJsonObject &, const QJsonObject &payload) {
        m_tokenEdit->setText(accessToken);
        m_client->setAccessToken(accessToken);
        m_overviewText->setPlainText(QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Indented)));
        appendLog(QStringLiteral("登录成功"));
        statusBar()->showMessage(QStringLiteral("已登录"), 3000);
    });
    connect(m_client, &ControlPlaneClient::nodesReady, this, [this](const QJsonArray &items, const QJsonObject &payload) {
        updateNodesTable(items);
        m_overviewText->setPlainText(QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Indented)));
        appendLog(QStringLiteral("已加载 %1 个节点").arg(items.size()));
        statusBar()->showMessage(QStringLiteral("节点已刷新"), 3000);
    });
    connect(m_client, &ControlPlaneClient::deviceRegistered, this, [this](const QJsonObject &payload) {
        m_registrationText->setPlainText(QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Indented)));
        appendLog(QStringLiteral("设备注册成功"));
        statusBar()->showMessage(QStringLiteral("设备已注册"), 3000);
    });
    connect(m_client, &ControlPlaneClient::requestFailed, this, [this](const QString &operation, const QString &message, int statusCode) {
        QString displayOperation = operation;
        if (operation == QStringLiteral("health")) {
            displayOperation = QStringLiteral("健康检查");
        } else if (operation == QStringLiteral("login")) {
            displayOperation = QStringLiteral("登录");
        } else if (operation == QStringLiteral("nodes")) {
            displayOperation = QStringLiteral("节点查询");
        } else if (operation == QStringLiteral("register")) {
            displayOperation = QStringLiteral("设备注册");
        }
        const QString line = QStringLiteral("%1失败（%2）：%3").arg(displayOperation).arg(statusCode).arg(message);
        appendLog(line);
        statusBar()->showMessage(line, 5000);
    });
}

void MainWindow::loadSettings() {
    const QString serverUrl = m_settings.value(QStringLiteral("serverUrl"), QStringLiteral("http://127.0.0.1:8080")).toString();
    m_serverUrlEdit->setText(serverUrl);
    m_emailEdit->setText(m_settings.value(QStringLiteral("email")).toString());
    m_tokenEdit->setText(m_settings.value(QStringLiteral("accessToken")).toString());
    m_registrationTokenEdit->setText(m_settings.value(QStringLiteral("registrationToken"), QStringLiteral("dev-register-token")).toString());
    m_deviceNameEdit->setText(m_settings.value(QStringLiteral("deviceName"), QStringLiteral("desktop-qt")).toString());
    m_platformEdit->setText(m_settings.value(QStringLiteral("platform"), QStringLiteral("windows-desktop")).toString());
    m_versionEdit->setText(m_settings.value(QStringLiteral("version"), QStringLiteral("0.1.0")).toString());
    m_publicKeyEdit->setText(m_settings.value(QStringLiteral("publicKey"), QStringLiteral("desktop-qt-devpub")).toString());
    m_capabilitiesEdit->setText(m_settings.value(QStringLiteral("capabilities"), QStringLiteral("desktop,gui,qt")).toString());

    m_serialTransportEdit->setText(m_settings.value(QStringLiteral("serialTransport"), QStringLiteral("tcp-encap")).toString());
    m_serialBaudRateEdit->setText(m_settings.value(QStringLiteral("serialBaudRate"), QStringLiteral("115200")).toString());
    m_usbTransportEdit->setText(m_settings.value(QStringLiteral("usbTransport"), QStringLiteral("usbip-encap")).toString());

    loadForwardingSettings();
    refreshLocalSerialDevices();
    refreshLocalUsbDevices();

    m_client->setBaseUrl(QUrl::fromUserInput(serverUrl));
    m_client->setAccessToken(m_tokenEdit->text().trimmed());
}

void MainWindow::saveSettings() {
    m_settings.setValue(QStringLiteral("serverUrl"), m_serverUrlEdit->text().trimmed());
    m_settings.setValue(QStringLiteral("email"), m_emailEdit->text().trimmed());
    m_settings.setValue(QStringLiteral("accessToken"), m_tokenEdit->text().trimmed());
    m_settings.setValue(QStringLiteral("registrationToken"), m_registrationTokenEdit->text().trimmed());
    m_settings.setValue(QStringLiteral("deviceName"), m_deviceNameEdit->text().trimmed());
    m_settings.setValue(QStringLiteral("platform"), m_platformEdit->text().trimmed());
    m_settings.setValue(QStringLiteral("version"), m_versionEdit->text().trimmed());
    m_settings.setValue(QStringLiteral("publicKey"), m_publicKeyEdit->text().trimmed());
    m_settings.setValue(QStringLiteral("capabilities"), m_capabilitiesEdit->text().trimmed());
    m_settings.setValue(QStringLiteral("serialTransport"), m_serialTransportEdit->text().trimmed());
    m_settings.setValue(QStringLiteral("serialBaudRate"), m_serialBaudRateEdit->text().trimmed());
    m_settings.setValue(QStringLiteral("usbTransport"), m_usbTransportEdit->text().trimmed());
    saveForwardingSettings();
}

void MainWindow::loadForwardingSettings() {
    const QByteArray serialRaw = m_settings.value(QStringLiteral("serialEntries")).toByteArray();
    const QJsonDocument serialDoc = QJsonDocument::fromJson(serialRaw);
    if (serialDoc.isArray()) {
        m_serialEntries = serialDoc.array();
    } else {
        m_serialEntries = QJsonArray();
    }

    const QByteArray usbRaw = m_settings.value(QStringLiteral("usbEntries")).toByteArray();
    const QJsonDocument usbDoc = QJsonDocument::fromJson(usbRaw);
    if (usbDoc.isArray()) {
        m_usbEntries = usbDoc.array();
    } else {
        m_usbEntries = QJsonArray();
    }

    refreshSerialTable();
    refreshUsbTable();
}

void MainWindow::saveForwardingSettings() {
    m_settings.setValue(QStringLiteral("serialEntries"), QJsonDocument(m_serialEntries).toJson(QJsonDocument::Compact));
    m_settings.setValue(QStringLiteral("usbEntries"), QJsonDocument(m_usbEntries).toJson(QJsonDocument::Compact));
}

void MainWindow::appendLog(const QString &line) {
    const QString timestamped = QStringLiteral("[%1] %2")
                                    .arg(QDateTime::currentDateTimeUtc().toString(Qt::ISODate))
                                    .arg(line);
    m_logText->appendPlainText(timestamped);
}

void MainWindow::refreshLocalSerialDevices() {
    m_localSerialDevices = LocalDeviceInventory::enumerateSerialDevices();
    m_serialDetectedCombo->clear();
    for (const QJsonValue &value : m_localSerialDevices) {
        const QJsonObject object = value.toObject();
        m_serialDetectedCombo->addItem(object.value(QStringLiteral("display_name")).toString(), object);
    }
    updateSerialRulePreview();
    applySelectedSerialDevice(true);
    appendLog(QStringLiteral("扫描到 %1 个本机串口设备").arg(m_localSerialDevices.size()));
}

void MainWindow::refreshLocalUsbDevices() {
    m_localUsbDevices = LocalDeviceInventory::enumerateUsbDevices();
    m_usbDetectedCombo->clear();
    for (const QJsonValue &value : m_localUsbDevices) {
        const QJsonObject object = value.toObject();
        m_usbDetectedCombo->addItem(object.value(QStringLiteral("display_name")).toString(), object);
    }
    updateUsbRulePreview();
    applySelectedUsbDevice(true);
    appendLog(QStringLiteral("扫描到 %1 个本机 USB 设备").arg(m_localUsbDevices.size()));
}

void MainWindow::applySelectedSerialDevice(bool onlyIfEmpty) {
    if (m_serialDetectedCombo->currentIndex() < 0) {
        updateSerialRulePreview();
        return;
    }
    if (onlyIfEmpty && !m_serialLocalPortEdit->text().trimmed().isEmpty()) {
        updateSerialRulePreview();
        return;
    }
    const QJsonObject object = m_serialDetectedCombo->currentData().toJsonObject();
    m_serialLocalPortEdit->setText(object.value(QStringLiteral("port_name")).toString());
    if (onlyIfEmpty && m_serialBaudRateEdit->text().trimmed().isEmpty()) {
        m_serialBaudRateEdit->setText(QString::number(object.value(QStringLiteral("suggested_baud_rate")).toInt(115200)));
    } else if (!onlyIfEmpty) {
        m_serialBaudRateEdit->setText(QString::number(object.value(QStringLiteral("suggested_baud_rate")).toInt(115200)));
    }
    if (onlyIfEmpty && m_serialTransportEdit->text().trimmed().isEmpty()) {
        m_serialTransportEdit->setText(object.value(QStringLiteral("transport")).toString(QStringLiteral("tcp-encap")));
    } else if (!onlyIfEmpty) {
        m_serialTransportEdit->setText(object.value(QStringLiteral("transport")).toString(QStringLiteral("tcp-encap")));
    }
    updateSerialRulePreview();
}

void MainWindow::applySelectedUsbDevice(bool onlyIfEmpty) {
    if (m_usbDetectedCombo->currentIndex() < 0) {
        updateUsbRulePreview();
        return;
    }
    if (onlyIfEmpty && (!m_usbLocalVendorEdit->text().trimmed().isEmpty() || !m_usbLocalBusEdit->text().trimmed().isEmpty())) {
        updateUsbRulePreview();
        return;
    }
    const QJsonObject object = m_usbDetectedCombo->currentData().toJsonObject();
    m_usbLocalBusEdit->setText(object.value(QStringLiteral("bus_id")).toString());
    m_usbLocalDeviceEdit->setText(object.value(QStringLiteral("device_id")).toString());
    m_usbLocalVendorEdit->setText(object.value(QStringLiteral("vendor_id")).toString());
    m_usbLocalProductEdit->setText(object.value(QStringLiteral("product_id")).toString());
    m_usbLocalInterfaceEdit->setText(object.value(QStringLiteral("interface")).toString());
    if (onlyIfEmpty && m_usbTransportEdit->text().trimmed().isEmpty()) {
        m_usbTransportEdit->setText(object.value(QStringLiteral("transport")).toString(QStringLiteral("usbip-encap")));
    } else if (!onlyIfEmpty) {
        m_usbTransportEdit->setText(object.value(QStringLiteral("transport")).toString(QStringLiteral("usbip-encap")));
    }
    updateUsbRulePreview();
}

void MainWindow::updateSerialRulePreview() {
    if (m_serialDetectedCombo->currentIndex() < 0) {
        m_serialRuleText->clear();
        return;
    }
    const QJsonObject object = m_serialDetectedCombo->currentData().toJsonObject();
    const QJsonObject preview{
        {QStringLiteral("display_name"), object.value(QStringLiteral("display_name")).toString()},
        {QStringLiteral("port_name"), object.value(QStringLiteral("port_name")).toString()},
        {QStringLiteral("driver"), object.value(QStringLiteral("driver")).toString()},
        {QStringLiteral("transport"), object.value(QStringLiteral("transport")).toString()},
        {QStringLiteral("suggested_baud_rate"), object.value(QStringLiteral("suggested_baud_rate")).toInt()},
        {QStringLiteral("rule"), object.value(QStringLiteral("rule")).toString()},
    };
    m_serialRuleText->setPlainText(QString::fromUtf8(QJsonDocument(preview).toJson(QJsonDocument::Indented)));
}

void MainWindow::updateUsbRulePreview() {
    if (m_usbDetectedCombo->currentIndex() < 0) {
        m_usbRuleText->clear();
        return;
    }
    const QJsonObject object = m_usbDetectedCombo->currentData().toJsonObject();
    const QJsonObject preview{
        {QStringLiteral("display_name"), object.value(QStringLiteral("display_name")).toString()},
        {QStringLiteral("vendor_id"), object.value(QStringLiteral("vendor_id")).toString()},
        {QStringLiteral("product_id"), object.value(QStringLiteral("product_id")).toString()},
        {QStringLiteral("driver"), object.value(QStringLiteral("driver")).toString()},
        {QStringLiteral("transport"), object.value(QStringLiteral("transport")).toString()},
        {QStringLiteral("rule"), object.value(QStringLiteral("rule")).toString()},
    };
    m_usbRuleText->setPlainText(QString::fromUtf8(QJsonDocument(preview).toJson(QJsonDocument::Indented)));
}

void MainWindow::updateNodesTable(const QJsonArray &items) {
    m_nodesTable->setRowCount(items.size());
    for (int row = 0; row < items.size(); ++row) {
        const QJsonObject item = items.at(row).toObject();
        const QJsonArray endpoints = item.value(QStringLiteral("endpoints")).toArray();
        QStringList endpointValues;
        for (const QJsonValue &endpoint : endpoints) {
            endpointValues.append(endpoint.toString());
        }
        const QStringList columns{
            item.value(QStringLiteral("id")).toString(),
            item.value(QStringLiteral("device_id")).toString(),
            item.value(QStringLiteral("overlay_ip")).toString(),
            item.value(QStringLiteral("status")).toString(),
            item.value(QStringLiteral("relay_region")).toString(),
            item.value(QStringLiteral("last_seen_at")).toString(),
            endpointValues.join(QStringLiteral(", ")),
        };
        for (int column = 0; column < columns.size(); ++column) {
            auto *cell = new QTableWidgetItem(columns.at(column));
            m_nodesTable->setItem(row, column, cell);
        }
    }
}

void MainWindow::refreshSerialTable() {
    m_serialTable->setRowCount(m_serialEntries.size());
    for (int row = 0; row < m_serialEntries.size(); ++row) {
        const QJsonObject item = m_serialEntries.at(row).toObject();
        const QJsonObject local = item.value(QStringLiteral("local")).toObject();
        const QJsonObject remote = item.value(QStringLiteral("remote")).toObject();
        const QStringList columns{
            item.value(QStringLiteral("session_id")).toString(),
            item.value(QStringLiteral("node_id")).toString(),
            item.value(QStringLiteral("peer_node_id")).toString(),
            local.value(QStringLiteral("name")).toString(),
            remote.value(QStringLiteral("name")).toString(),
            QString::number(local.value(QStringLiteral("baud_rate")).toInt()),
            item.value(QStringLiteral("transport")).toString(),
        };
        for (int column = 0; column < columns.size(); ++column) {
            auto *cell = new QTableWidgetItem(columns.at(column));
            m_serialTable->setItem(row, column, cell);
        }
    }
    syncForwardingPreview(m_serialJsonText, m_serialEntries);
}

void MainWindow::refreshUsbTable() {
    m_usbTable->setRowCount(m_usbEntries.size());
    for (int row = 0; row < m_usbEntries.size(); ++row) {
        const QJsonObject item = m_usbEntries.at(row).toObject();
        const QJsonObject local = item.value(QStringLiteral("local")).toObject();
        const QJsonObject remote = item.value(QStringLiteral("remote")).toObject();
        const QString localLabel = QStringLiteral("%1/%2 %3:%4")
                                       .arg(local.value(QStringLiteral("bus_id")).toString(),
                                            local.value(QStringLiteral("device_id")).toString(),
                                            local.value(QStringLiteral("vendor_id")).toString(),
                                            local.value(QStringLiteral("product_id")).toString());
        const QString remoteLabel = QStringLiteral("%1/%2 %3:%4")
                                        .arg(remote.value(QStringLiteral("bus_id")).toString(),
                                             remote.value(QStringLiteral("device_id")).toString(),
                                             remote.value(QStringLiteral("vendor_id")).toString(),
                                             remote.value(QStringLiteral("product_id")).toString());
        const QStringList columns{
            item.value(QStringLiteral("session_id")).toString(),
            item.value(QStringLiteral("node_id")).toString(),
            item.value(QStringLiteral("peer_node_id")).toString(),
            localLabel.trimmed(),
            remoteLabel.trimmed(),
            item.value(QStringLiteral("transport")).toString(),
        };
        for (int column = 0; column < columns.size(); ++column) {
            auto *cell = new QTableWidgetItem(columns.at(column));
            m_usbTable->setItem(row, column, cell);
        }
    }
    syncForwardingPreview(m_usbJsonText, m_usbEntries);
}

void MainWindow::syncForwardingPreview(QPlainTextEdit *editor, const QJsonArray &entries) {
    editor->setPlainText(QString::fromUtf8(QJsonDocument(entries).toJson(QJsonDocument::Indented)));
}

void MainWindow::exportEntries(const QString &title, const QString &suggestedName, const QJsonArray &entries) {
    const QString path = QFileDialog::getSaveFileName(this, title, suggestedName, QStringLiteral("JSON 文件 (*.json)"));
    if (path.isEmpty()) {
        return;
    }
    QFile file(path);
    if (!file.open(QIODevice::WriteOnly | QIODevice::Truncate)) {
        appendLog(QStringLiteral("导出失败：%1").arg(path));
        return;
    }
    file.write(QJsonDocument(entries).toJson(QJsonDocument::Indented));
    appendLog(QStringLiteral("已导出转发配置到 %1").arg(path));
}

void MainWindow::exportAgentConfigSnippet(const QString &platform, const QString &suggestedName) {
    const QString title = platform == QStringLiteral("windows-agent")
                              ? QStringLiteral("导出 Windows Agent 转发配置片段")
                              : QStringLiteral("导出 Linux Agent 转发配置片段");
    const QString path = QFileDialog::getSaveFileName(this, title, suggestedName, QStringLiteral("JSON 文件 (*.json)"));
    if (path.isEmpty()) {
        return;
    }

    QFile file(path);
    if (!file.open(QIODevice::WriteOnly | QIODevice::Truncate)) {
        appendLog(QStringLiteral("导出失败：%1").arg(path));
        return;
    }
    file.write(QJsonDocument(buildAgentConfigSnippet(platform)).toJson(QJsonDocument::Indented));
    appendLog(QStringLiteral("已导出 %1 转发配置片段到 %2").arg(platform, path));
}

void MainWindow::importAgentConfigSnippet(const QString &title) {
    const QString path = QFileDialog::getOpenFileName(this, title, QString(), QStringLiteral("JSON 文件 (*.json)"));
    if (path.isEmpty()) {
        return;
    }
    QFile file(path);
    if (!file.open(QIODevice::ReadOnly)) {
        appendLog(QStringLiteral("打开失败：%1").arg(path));
        return;
    }
    const QJsonDocument doc = QJsonDocument::fromJson(file.readAll());
    if (!doc.isObject()) {
        appendLog(QStringLiteral("配置片段格式不正确：%1").arg(path));
        return;
    }
    const QJsonObject object = doc.object();
    const QJsonArray serialEntries = object.value(QStringLiteral("serial_forwards")).toArray();
    const QJsonArray usbEntries = object.value(QStringLiteral("usb_forwards")).toArray();

    if (!serialEntries.isEmpty()) {
        m_serialEntries = serialEntries;
        refreshSerialTable();
    }
    if (!usbEntries.isEmpty()) {
        m_usbEntries = usbEntries;
        refreshUsbTable();
    }
    if (!serialEntries.isEmpty() || !usbEntries.isEmpty()) {
        saveForwardingSettings();
        appendLog(QStringLiteral("已从 %1 导入 Agent 转发配置片段").arg(path));
        return;
    }
    appendLog(QStringLiteral("在 %1 中未找到 serial_forwards 或 usb_forwards").arg(path));
}

bool MainWindow::importEntriesFromFile(const QString &title, const QString &objectKey, QJsonArray *entries) {
    const QString path = QFileDialog::getOpenFileName(this, title, QString(), QStringLiteral("JSON 文件 (*.json)"));
    if (path.isEmpty()) {
        return false;
    }
    QFile file(path);
    if (!file.open(QIODevice::ReadOnly)) {
        appendLog(QStringLiteral("打开失败：%1").arg(path));
        return false;
    }
    const QJsonDocument doc = QJsonDocument::fromJson(file.readAll());
    QJsonArray imported;
    if (doc.isArray()) {
        imported = doc.array();
    } else if (doc.isObject()) {
        imported = doc.object().value(objectKey).toArray();
    }
    if (imported.isEmpty()) {
        appendLog(QStringLiteral("在 %2 中未找到 %1 条目").arg(objectKey, path));
        return false;
    }
    *entries = imported;
    appendLog(QStringLiteral("已从 %2 导入 %1 条目").arg(path, objectKey));
    return true;
}

void MainWindow::loadJsonPreviewFromFile(const QString &title, QPlainTextEdit *editor) {
    const QString path = QFileDialog::getOpenFileName(this, title, QString(), QStringLiteral("JSON 文件 (*.json)"));
    if (path.isEmpty()) {
        return;
    }
    QFile file(path);
    if (!file.open(QIODevice::ReadOnly)) {
        appendLog(QStringLiteral("打开失败：%1").arg(path));
        return;
    }
    const QJsonDocument doc = QJsonDocument::fromJson(file.readAll());
    if (doc.isNull()) {
        appendLog(QStringLiteral("JSON 解析失败：%1").arg(path));
        return;
    }
    editor->setPlainText(QString::fromUtf8(doc.toJson(QJsonDocument::Indented)));
    appendLog(QStringLiteral("已加载报告 %1").arg(path));
}

QJsonObject MainWindow::buildRegistrationPayload() const {
    QJsonArray capabilities;
    const QStringList parts = m_capabilitiesEdit->text().split(',', Qt::SkipEmptyParts);
    for (const QString &part : parts) {
        capabilities.append(part.trimmed());
    }

    return QJsonObject{
        {QStringLiteral("device_name"), m_deviceNameEdit->text().trimmed()},
        {QStringLiteral("platform"), m_platformEdit->text().trimmed()},
        {QStringLiteral("version"), m_versionEdit->text().trimmed()},
        {QStringLiteral("public_key"), m_publicKeyEdit->text().trimmed()},
        {QStringLiteral("registration_token"), m_registrationTokenEdit->text().trimmed()},
        {QStringLiteral("capabilities"), capabilities},
    };
}

QJsonObject MainWindow::buildSerialPayload() const {
    const QString nodeId = m_serialNodeIdEdit->text().trimmed();
    const QString peerNodeId = m_serialPeerNodeIdEdit->text().trimmed();
    const QString localPort = m_serialLocalPortEdit->text().trimmed();
    const QString remotePort = m_serialRemotePortEdit->text().trimmed();
    const QString transport = m_serialTransportEdit->text().trimmed().isEmpty() ? QStringLiteral("tcp-encap") : m_serialTransportEdit->text().trimmed();
    const int baudRate = m_serialBaudRateEdit->text().trimmed().isEmpty() ? 115200 : m_serialBaudRateEdit->text().trimmed().toInt();

    if (localPort.isEmpty() || remotePort.isEmpty()) {
        return QJsonObject();
    }

    return QJsonObject{
        {QStringLiteral("session_id"), buildForwardingSessionId(QStringLiteral("serial"), nodeId, peerNodeId, localPort, remotePort, transport)},
        {QStringLiteral("node_id"), nodeId},
        {QStringLiteral("peer_node_id"), peerNodeId},
        {QStringLiteral("transport"), transport},
        {QStringLiteral("local"), QJsonObject{
             {QStringLiteral("name"), localPort},
             {QStringLiteral("baud_rate"), baudRate},
             {QStringLiteral("data_bits"), 8},
             {QStringLiteral("stop_bits"), 1},
             {QStringLiteral("parity"), QStringLiteral("none")},
             {QStringLiteral("read_timeout_millis"), 1000},
         }},
        {QStringLiteral("remote"), QJsonObject{
             {QStringLiteral("name"), remotePort},
             {QStringLiteral("baud_rate"), baudRate},
             {QStringLiteral("data_bits"), 8},
             {QStringLiteral("stop_bits"), 1},
             {QStringLiteral("parity"), QStringLiteral("none")},
             {QStringLiteral("read_timeout_millis"), 1000},
         }},
    };
}

QJsonObject MainWindow::buildUsbPayload() const {
    const QString nodeId = m_usbNodeIdEdit->text().trimmed();
    const QString peerNodeId = m_usbPeerNodeIdEdit->text().trimmed();
    const QString transport = m_usbTransportEdit->text().trimmed().isEmpty() ? QStringLiteral("usbip-encap") : m_usbTransportEdit->text().trimmed();
    const QString localId = QStringLiteral("%1-%2-%3-%4-%5")
                                .arg(m_usbLocalBusEdit->text().trimmed(),
                                     m_usbLocalDeviceEdit->text().trimmed(),
                                     m_usbLocalVendorEdit->text().trimmed(),
                                     m_usbLocalProductEdit->text().trimmed(),
                                     m_usbLocalInterfaceEdit->text().trimmed());
    const QString remoteId = QStringLiteral("%1-%2-%3-%4-%5")
                                 .arg(m_usbRemoteBusEdit->text().trimmed(),
                                      m_usbRemoteDeviceEdit->text().trimmed(),
                                      m_usbRemoteVendorEdit->text().trimmed(),
                                      m_usbRemoteProductEdit->text().trimmed(),
                                      m_usbRemoteInterfaceEdit->text().trimmed());

    const bool hasLocal = !m_usbLocalBusEdit->text().trimmed().isEmpty() ||
                          !m_usbLocalDeviceEdit->text().trimmed().isEmpty() ||
                          !m_usbLocalVendorEdit->text().trimmed().isEmpty() ||
                          !m_usbLocalProductEdit->text().trimmed().isEmpty();
    const bool hasRemote = !m_usbRemoteBusEdit->text().trimmed().isEmpty() ||
                           !m_usbRemoteDeviceEdit->text().trimmed().isEmpty() ||
                           !m_usbRemoteVendorEdit->text().trimmed().isEmpty() ||
                           !m_usbRemoteProductEdit->text().trimmed().isEmpty();
    if (!hasLocal || !hasRemote) {
        return QJsonObject();
    }

    return QJsonObject{
        {QStringLiteral("session_id"), buildForwardingSessionId(QStringLiteral("usb"), nodeId, peerNodeId, localId, remoteId, transport)},
        {QStringLiteral("node_id"), nodeId},
        {QStringLiteral("peer_node_id"), peerNodeId},
        {QStringLiteral("transport"), transport},
        {QStringLiteral("local"), QJsonObject{
             {QStringLiteral("bus_id"), m_usbLocalBusEdit->text().trimmed()},
             {QStringLiteral("device_id"), m_usbLocalDeviceEdit->text().trimmed()},
             {QStringLiteral("vendor_id"), m_usbLocalVendorEdit->text().trimmed()},
             {QStringLiteral("product_id"), m_usbLocalProductEdit->text().trimmed()},
             {QStringLiteral("interface"), m_usbLocalInterfaceEdit->text().trimmed()},
         }},
        {QStringLiteral("remote"), QJsonObject{
             {QStringLiteral("bus_id"), m_usbRemoteBusEdit->text().trimmed()},
             {QStringLiteral("device_id"), m_usbRemoteDeviceEdit->text().trimmed()},
             {QStringLiteral("vendor_id"), m_usbRemoteVendorEdit->text().trimmed()},
             {QStringLiteral("product_id"), m_usbRemoteProductEdit->text().trimmed()},
             {QStringLiteral("interface"), m_usbRemoteInterfaceEdit->text().trimmed()},
         }},
    };
}

QJsonObject MainWindow::buildAgentConfigSnippet(const QString &platform) const {
    QJsonObject object{
        {QStringLiteral("server_url"), m_serverUrlEdit->text().trimmed()},
        {QStringLiteral("platform"), platform},
        {QStringLiteral("serial_forwards"), m_serialEntries},
        {QStringLiteral("usb_forwards"), m_usbEntries},
    };
    if (platform == QStringLiteral("windows-agent")) {
        object.insert(QStringLiteral("serial_forward_path"), QStringLiteral("./data/windows-agent-serial-forwards.json"));
        object.insert(QStringLiteral("serial_forward_report_path"), QStringLiteral("./data/windows-agent-serial-forward-report.json"));
        object.insert(QStringLiteral("usb_forward_path"), QStringLiteral("./data/windows-agent-usb-forwards.json"));
        object.insert(QStringLiteral("usb_forward_report_path"), QStringLiteral("./data/windows-agent-usb-forward-report.json"));
    } else {
        object.insert(QStringLiteral("serial_forward_path"), QStringLiteral("/var/lib/nodeweave/linux-agent/serial-forwards.json"));
        object.insert(QStringLiteral("serial_forward_report_path"), QStringLiteral("/var/lib/nodeweave/linux-agent/serial-forward-report.json"));
        object.insert(QStringLiteral("usb_forward_path"), QStringLiteral("/var/lib/nodeweave/linux-agent/usb-forwards.json"));
        object.insert(QStringLiteral("usb_forward_report_path"), QStringLiteral("/var/lib/nodeweave/linux-agent/usb-forward-report.json"));
    }
    return object;
}

QString MainWindow::buildForwardingSessionId(const QString &prefix,
                                             const QString &nodeId,
                                             const QString &peerNodeId,
                                             const QString &left,
                                             const QString &right,
                                             const QString &transport) const {
    return QStringLiteral("%1-%2-%3-%4-%5-%6")
        .arg(prefix,
             sanitizeIdPart(nodeId),
             sanitizeIdPart(peerNodeId),
             sanitizeIdPart(left),
             sanitizeIdPart(right),
             sanitizeIdPart(transport));
}

QString MainWindow::sanitizeIdPart(const QString &value) {
    QString sanitized = value.trimmed();
    if (sanitized.isEmpty()) {
        return QStringLiteral("unknown");
    }
    sanitized.replace('\\', '_');
    sanitized.replace('/', '_');
    sanitized.replace(':', '_');
    sanitized.replace(' ', '_');
    return sanitized;
}
